// astrbox-train exports gate training samples and validates MLP weights.
//
// Training itself lives in the 算法导论 repository (training/mlp_jsonl); this
// command only turns recorded gate decisions into {"x":[...],"y":int} rows and
// checks that a weights file matches the Neko gate contract (see docs/model-gate.md).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/learn/mlp"
	"github.com/RuiCat/Neko/internal/learn/samples"
)

// TRAIN_FILE 是导出的 MLP 训练集文件名（放在 <data-dir>/training/ 下）。
const TRAIN_FILE = "gate-v1.jsonl"

func main() {
	dataDir := flag.String("data-dir", "data", "运行数据目录（journal/training/models 都在这里）")
	samplesPath := flag.String("samples", "", "门控样本文件（默认 <data-dir>/training/gate-samples.jsonl）")
	outPath := flag.String("out", "", "训练集输出（默认 <data-dir>/training/gate-v1.jsonl）")
	checkModel := flag.String("check-model", "", "只校验一份权重 JSON 是否符合门控契约（不导出）")
	flag.Parse()

	if *checkModel != "" {
		if err := checkModelFile(*checkModel); err != nil {
			log.Fatalf("astrbox-train: %v", err)
		}
		fmt.Printf("OK: %s 可被门控加载（输入 %d 维 / 输出 %d 类语义专家）\n", *checkModel, gate.FeatureSizeV1, len(gate.SEMANTIC_EXPERTS))
		return
	}

	outFile := resolve(*outPath, *dataDir, TRAIN_FILE)
	records, err := readRecords(*samplesPath, *dataDir)
	if err != nil {
		log.Fatalf("astrbox-train: %v", err)
	}
	if len(records) == 0 {
		log.Fatalf("astrbox-train: 没有样本（%s 与 %s）。先让 dsh-mcp 跑几轮，或用 gate_teach 提交教师样本",
			samples.Path(*dataDir, samples.SAMPLES_FILE), samples.Path(*dataDir, samples.TEACHER_FILE))
	}
	if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
		log.Fatalf("astrbox-train: 创建输出目录失败: %v", err)
	}
	file, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("astrbox-train: 创建训练集失败: %v", err)
	}
	kept, skipped, err := samples.Export(file, records)
	closeErr := file.Close()
	if err != nil {
		log.Fatalf("astrbox-train: %v", err)
	}
	if closeErr != nil {
		log.Fatalf("astrbox-train: 关闭训练集失败: %v", closeErr)
	}
	fmt.Printf("导出完成: %s\n样本 %d 条（保留 %d / 跳过 %d，跳过大都没有可用的语义专家标签）\n", outFile, len(records), kept, skipped)
	printDistribution(records)
	fmt.Printf("下一步: 在算法导论仓库跑 training/mlp_jsonl -data %s -out <weights.json>，把 weights 放到 <data-dir>/models/gate-mlp-v1.json 后重启 dsh-mcp\n", outFile)
}

// readRecords 读取训练样本：显式 -samples 只读该文件，否则自动样本与教师样本合并。
func readRecords(explicit, dataDir string) ([]samples.Record, error) {
	if explicit != "" {
		return samples.ReadFile(explicit)
	}
	return samples.ReadAll(dataDir)
}

// resolve 解析默认路径（相对 data-dir 的 training/ 子目录）。
func resolve(explicit, dataDir, name string) string {
	if explicit != "" {
		return explicit
	}
	return filepath.Join(dataDir, samples.SAMPLES_DIR, name)
}

// checkModelFile 校验权重文件与门控契约（输入维数、类别数），并打印网络形状。
func checkModelFile(path string) error {
	model, err := mlp.LoadFile(path)
	if err != nil {
		return err
	}
	if model.InputSize() != gate.FeatureSizeV1 {
		return fmt.Errorf("模型输入 %d 维，门控特征要求 %d 维（特征契约 v1）", model.InputSize(), gate.FeatureSizeV1)
	}
	if model.NumClasses() != len(gate.SEMANTIC_EXPERTS) {
		return fmt.Errorf("模型输出 %d 类，门控要求 %d 类（SEMANTIC_EXPERTS）", model.NumClasses(), len(gate.SEMANTIC_EXPERTS))
	}
	fmt.Printf("模型: layers=%v activation=%s\n", model.Layers(), model.Activation())
	return nil
}

// printDistribution 打印样本的主专家分布（快速看数据是否偏斜）。
func printDistribution(records []samples.Record) {
	counts := make(map[string]int, len(gate.SEMANTIC_EXPERTS))
	skipped := 0
	for _, record := range records {
		label := samples.LabelFor(record)
		if label < 0 {
			skipped++
			continue
		}
		counts[gate.SEMANTIC_EXPERTS[label]]++
	}
	fmt.Print("主专家标签分布: ")
	for _, expert := range gate.SEMANTIC_EXPERTS {
		fmt.Printf("%s=%d ", expert, counts[expert])
	}
	fmt.Printf("(无标签 %d)\n", skipped)
}
