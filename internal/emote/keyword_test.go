package emote

import "testing"

// keywordDataSet builds rows with distinct category/description shapes.
func keywordDataSet() *DataSet {
	order := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg", "f.jpg"}
	index := map[string]IndexEntry{
		"a.jpg": {Desc: "开心地笑", Categories: []string{"开心"}, Keywords: []string{"开心"}},
		"b.jpg": {Desc: "难过大哭", Categories: []string{"难过", "其他"}},
		"c.jpg": {Desc: "暧昧亲亲", Categories: []string{"暧昧", "爱"}},
		"d.jpg": {Desc: "日常早安", Categories: []string{"日常"}},
		"e.jpg": {Desc: "早安打卡", Categories: []string{"日常"}},
		"f.jpg": {Desc: "早安打卡", Categories: []string{"日常", "其他"}},
	}
	return &DataSet{Index: index, IndexOrder: order}
}

// TestUniqueHanWords mirrors Python's 2-4 Han-character regex.
func TestUniqueHanWords(t *testing.T) {
	got := uniqueHanWords("主人我好想你呀，早安")
	want := []string{"主人我好", "想你呀", "早安"}
	if len(got) != len(want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("words = %v, want %v", got, want)
		}
	}
}

// TestSearchKeywordBasic scores category and description matches.
func TestSearchKeywordBasic(t *testing.T) {
	results := searchKeyword(keywordDataSet(), "早安", 5)
	if len(results) == 0 || results[0].File != "d.jpg" {
		t.Fatalf("results = %+v, want d.jpg first", results)
	}
	for _, result := range results {
		if result.File == "c.jpg" {
			t.Fatal("暧昧 row leaked without an intimate query")
		}
	}
}

// TestSearchKeywordIntimateAllowsAmbiguous keeps 暧昧 rows for intimate text.
func TestSearchKeywordIntimateAllowsAmbiguous(t *testing.T) {
	results := searchKeyword(keywordDataSet(), "亲亲", 5)
	found := false
	for _, result := range results {
		if result.File == "c.jpg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("c.jpg should pass the intimate gate, got %+v", results)
	}
}

// TestSearchKeywordOtherPenalty ranks 其他 rows lower.
func TestSearchKeywordOtherPenalty(t *testing.T) {
	results := searchKeyword(keywordDataSet(), "早安", 5)
	if len(results) < 2 {
		t.Fatalf("results = %+v", results)
	}
	position := map[string]int{}
	for i, result := range results {
		position[result.File] = i
	}
	if position["e.jpg"] > position["f.jpg"] {
		t.Fatalf("其他 row ranked above the plain row: %v", results)
	}
}

// TestSearchKeywordEmptyText returns nothing.
func TestSearchKeywordEmptyText(t *testing.T) {
	if results := searchKeyword(keywordDataSet(), "   ", 3); results != nil {
		t.Fatalf("results = %+v, want nil", results)
	}
}
