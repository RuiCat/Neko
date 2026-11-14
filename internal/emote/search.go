// search.go reproduces the Python two-layer retrieval of emote_vec_db.search:
// query embedding → top-20 content clusters → candidate pool (cluster members
// plus intent keyword direct hits) → R18 filter, similarity threshold and
// intent-fusion score. The intent/negative/R18 tables are frozen copies; the
// acceptance baseline (verify_emote_negkw.py) depends on them.
package emote

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// intentKeywords mirrors INTENT_KW (16 intents, dict order preserved).
var intentKeywords = []struct {
	name     string
	keywords []string
}{
	{"开心", []string{"开心", "高兴", "哈哈", "好耶", "快乐", "笑", "得意", "庆祝", "兴奋", "欢呼", "笑嘻嘻", "眉开眼笑"}},
	{"难过", []string{"难过", "伤心", "哭", "呜呜", "委屈", "失落", "悲伤", "沮丧", "泪", "想哭", "泪目", "哭唧唧"}},
	{"生气", []string{"生气", "愤怒", "气死", "烦", "讨厌", "抓狂", "怒", "火", "气鼓鼓", "气呼呼"}},
	{"惊讶", []string{"惊讶", "震惊", "吃惊", "吓", "意外", "难以置信", "天哪", "惊恐", "目瞪口呆", "惊呆"}},
	{"害羞", []string{"害羞", "脸红", "不好意思", "羞涩", "腼腆", "红晕", "红着脸", "羞羞", "娇羞"}},
	{"撒娇", []string{"撒娇", "蹭", "卖萌", "可爱", "撒娇地", "扭来扭去", "蹭蹭", "打滚", "嘟嘴"}},
	{"抱抱", []string{"抱抱", "抱", "搂", "拥", "贴贴", "怀里", "抱住", "拥抱", "依偎"}},
	{"亲亲", []string{"亲亲", "亲", "吻", "么么", "啵", "啾", "mua", "亲一口", "亲一个"}},
	{"摸头", []string{"摸头", "摸摸", "揉揉", "摸摸头", "撸", "顺毛", "摸摸脑袋"}},
	{"爱", []string{"爱", "喜欢", "想你", "比心", "么么", "甜蜜", "心动", "爱心", "喜欢你", "最爱"}},
	{"困", []string{"困了", "睡觉", "打哈欠", "困倦", "疲惫", "累", "晚安", "休息", "犯困", "想睡", "睡衣", "睡着", "打瞌睡", "瞌睡", "好困", "困困", "困死了", "好想睡"}},
	{"疑问", []string{"疑问", "疑惑", "为什么", "什么", "好奇", "困惑", "问号", "怎么回事", "咋了"}},
	{"嫌弃", []string{"嫌弃", "无语", "翻白眼", "呵呵", "切", "鄙视", "无奈", "吐槽", "汗", "囧"}},
	{"加油", []string{"加油", "努力", "奋斗", "冲", "坚持", "鼓励", "打气"}},
	{"害怕", []string{"害怕", "怕", "恐怖", "吓死", "救命", "慌张", "恐惧", "惊恐", "打雷", "发抖", "瑟瑟发抖"}},
	{"吃货", []string{"吃", "饿", "蛋糕", "好吃", "零食", "甜品", "美食", "干饭", "西瓜", "食物", "肚子饿", "嘴馋", "馋"}},
}

// negativeKeywords mirrors NEG_KW (candidate-side disambiguation).
var negativeKeywords = map[string][]string{
	"抱抱": {"抱头", "抱胸", "抱枕", "抱婴儿", "抱着婴儿", "抱娃", "抱猫", "抱拳", "抱住自己", "抱腿", "抱着一", "抱着纸"},
	"亲亲": {"亲戚", "亲近", "亲爱的"},
	"摸头": {"摸鱼", "摸索", "摸底", "抚摸地面"},
	"爱":  {"爱好"},
	"吃货": {"吃饱"},
	"难过": {"难过地笑"},
}

// r18Keywords is the content filter that must never be relaxed (R6).
var r18Keywords = []string{
	"淫乱", "援交", "娇喘", "裸", "性", "色情", "高潮", "精液", "阴道", "阴茎",
	"做爱", "口交", "乳头", "生殖器", "露骨", "一丝不挂", "全裸", "性爱", "自慰",
	"勃起", "奶子", "胸部", "乳房", "强奸", "操哭", "操你", "色图", "本子", "里番",
}

// intimateKeywords gates the 暧昧 category (both vector tiers and keyword tier).
var intimateKeywords = []string{"亲亲", "抱抱", "想你", "喜欢你", "爱", "贴贴", "蹭", "亲", "吻", "么么"}

// detectIntents returns the intents whose keywords appear in the query, minus
// any intent whose negative scenario the query itself triggers.
func detectIntents(text string) []string {
	intents := make([]string, 0, 4)
	for _, intent := range intentKeywords {
		if !containsAny(text, intent.keywords) {
			continue
		}
		if containsAny(text, negativeKeywords[intent.name]) {
			continue
		}
		intents = append(intents, intent.name)
	}
	return intents
}

// scoredCandidate is one row of the intermediate ranking.
type scoredCandidate struct {
	score   float64
	index   int
	bonus   float64
	cluster int
}

// searchVecDB runs the clustered two-layer search (the primary tier).
func searchVecDB(ctx context.Context, data *DataSet, embedder Embedder, text string, top int) ([]Result, error) {
	if !data.HasDB || data.Dim <= 0 {
		return nil, fmt.Errorf("表情向量库未加载")
	}
	intents := detectIntents(text)
	query, err := embedder.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedding, err)
	}
	if len(query) != data.Dim {
		return nil, fmt.Errorf("%w: 查询 %d 维, 索引 %d 维（模型是否换过？）", ErrVectorDimension, len(query), data.Dim)
	}
	queryNormalized := normalizeVector(query)

	topClusters := nearestClusters(queryNormalized, data.Centers, data.Dim, 20)
	clusterSet := make(map[int]bool, len(topClusters))
	for _, cluster := range topClusters {
		clusterSet[cluster] = true
	}
	candidates := map[int]bool{}
	for row, label := range data.Labels {
		if clusterSet[int(label)] {
			candidates[row] = true
		}
	}
	intentHits := map[int]bool{}
	for _, intent := range intents {
		limit := top * 20
		added := 0
		for row := 0; row < len(data.Meta.Names) && added < limit; row++ {
			if candidates[row] {
				continue
			}
			name := data.name(row)
			entry, ok := data.Index[name]
			if !ok {
				continue
			}
			if containsAny(entry.Desc, intentKeywordsOf(intent)) || containsAny(strings.Join(entry.Categories, " "), intentKeywordsOf(intent)) {
				candidates[row] = true
				intentHits[row] = true
				added++
			}
		}
	}
	order := make([]int, 0, len(candidates))
	for row := range candidates {
		order = append(order, row)
	}
	sort.Ints(order)

	scored := make([]scoredCandidate, 0, len(order))
	for _, row := range order {
		name := data.name(row)
		entry := data.Index[name]
		desc := entry.Desc
		similarity := dotProduct(normalizeVector(data.row(row)), queryNormalized)
		cluster := int(data.Labels[row])
		if containsAny(desc, r18Keywords) {
			continue
		}
		isIntentHit := intentHits[row]
		if similarity < SIM_THRESHOLD && !isIntentHit {
			continue
		}
		analysis := data.analysisFor(cluster)
		textPool := desc + " " + analysis
		bonus := 0.0
		strong := 0.0
		for _, intent := range intents {
			keywords := intentKeywordsOf(intent)
			if containsAny(desc, negativeKeywords[intent]) {
				continue
			}
			if containsAny(textPool, keywords) {
				bonus += 0.5
			}
			if containsAny(desc, keywords) {
				strong += 0.3
			}
		}
		finalScore := similarity*0.5 + math.Min(bonus+strong, SCORE_CAP)*0.5
		scored = append(scored, scoredCandidate{score: finalScore, index: row, bonus: bonus + strong, cluster: cluster})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	return resultsFromCandidates(data, scored, top), nil
}

// intentKeywordsOf returns the keyword list of one intent name.
func intentKeywordsOf(name string) []string {
	for _, intent := range intentKeywords {
		if intent.name == name {
			return intent.keywords
		}
	}
	return nil
}

// resultsFromCandidates truncates the ranking and resolves image paths.
func resultsFromCandidates(data *DataSet, scored []scoredCandidate, top int) []Result {
	if len(scored) > top {
		scored = scored[:top]
	}
	results := make([]Result, 0, len(scored))
	for _, candidate := range scored {
		name := data.name(candidate.index)
		entry := data.Index[name]
		path, missing := data.pathFor(name)
		results = append(results, Result{
			File:        name,
			Desc:        truncateRunes(entry.Desc, 100),
			Score:       roundScore(candidate.score),
			Categories:  entry.Categories,
			IntentBonus: roundScore(candidate.bonus),
			Cluster:     candidate.cluster,
			Path:        path,
			Missing:     missing,
		})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// searchFlatVectors is the second tier: cosine over emote_vectors.json, with
// the 0.35 threshold and the 暧昧 filter of emote_matcher_v2.
func searchFlatVectors(ctx context.Context, data *DataSet, embedder Embedder, text string, top int) ([]Result, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	vectors, ok := data.loadVectors()
	if !ok {
		return nil, fmt.Errorf("表情向量文件缺失或损坏: %s", VECTORS_FILE_NAME)
	}
	query, err := embedder.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedding, err)
	}
	queryNormalized := normalizeVector(query)
	intimate := containsAny(text, intimateKeywords)
	type flatHit struct {
		file  string
		score float64
	}
	hits := make([]flatHit, 0, 64)
	for _, name := range data.IndexOrder {
		vector, ok := vectors[name]
		if !ok || len(vector) == 0 || len(vector) != len(query) {
			continue
		}
		entry := data.Index[name]
		if hasCategory(entry.Categories, "暧昧") && !intimate {
			continue
		}
		similarity := dotProduct(normalizeVector(vector), queryNormalized)
		if similarity >= SIM_THRESHOLD {
			hits = append(hits, flatHit{file: name, score: similarity})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > top {
		hits = hits[:top]
	}
	results := make([]Result, 0, len(hits))
	for _, hit := range hits {
		entry := data.Index[hit.file]
		path, missing := data.pathFor(hit.file)
		results = append(results, Result{
			File:        hit.file,
			Desc:        truncateRunes(entry.Desc, 100),
			Score:       roundScore(hit.score),
			Categories:  entry.Categories,
			IntentBonus: 0,
			Cluster:     -1,
			Path:        path,
			Missing:     missing,
		})
	}
	return results, nil
}

// nearestClusters returns the k cluster indexes with the highest cosine
// similarity to the normalized query (numpy argsort(-cos)[:k] equivalent).
func nearestClusters(query []float32, centers []float32, dim, k int) []int {
	if dim <= 0 || len(centers) < dim {
		return nil
	}
	count := len(centers) / dim
	type clusterScore struct {
		cluster int
		score   float64
	}
	scores := make([]clusterScore, count)
	for cluster := 0; cluster < count; cluster++ {
		scores[cluster] = clusterScore{cluster: cluster, score: dotProduct(normalizeVector(centers[cluster*dim:(cluster+1)*dim]), query)}
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	if k > len(scores) {
		k = len(scores)
	}
	top := make([]int, k)
	for i := 0; i < k; i++ {
		top[i] = scores[i].cluster
	}
	return top
}

// normalizeVector returns the unit vector (zero vectors stay zero).
func normalizeVector(vector []float32) []float32 {
	norm := math.Sqrt(dotProduct(vector, vector))
	normalized := make([]float32, len(vector))
	if norm == 0 {
		return normalized
	}
	for i, value := range vector {
		normalized[i] = float32(float64(value) / norm)
	}
	return normalized
}

// dotProduct accumulates in float64 over float32 inputs.
func dotProduct(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	sum := 0.0
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// hasCategory reports whether the category list contains the value.
func hasCategory(categories []string, value string) bool {
	for _, category := range categories {
		if category == value {
			return true
		}
	}
	return false
}

// clusterName renders a cluster id the way cluster_analysis.json keys it.
func clusterName(cluster int) string {
	return strconv.Itoa(cluster)
}
