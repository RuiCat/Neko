// reader.go reads journal JSONL files back for the offline metrics pass and
// seeds the snapshot sequence after a restart.
package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ReadDir reads every *.jsonl file in dir and returns records ordered by start
// time (ties broken by turn ID). A malformed line fails loudly with file:line.
func ReadDir(dir string) ([]TurnRecord, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("journal: 扫描 %s 失败: %w", dir, err)
	}
	sort.Strings(paths)
	records := make([]TurnRecord, 0, len(paths))
	for _, path := range paths {
		fileRecords, err := readFile(path)
		if err != nil {
			return nil, err
		}
		records = append(records, fileRecords...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].StartedAt.Equal(records[j].StartedAt) {
			return records[i].TurnID < records[j].TurnID
		}
		return records[i].StartedAt.Before(records[j].StartedAt)
	})
	return records, nil
}

// MaxSequence scans the file of day and returns the highest sequence number of
// any evt_<yyyymmdd>_<seq> turn ID. A missing file means zero.
func MaxSequence(dir string, day time.Time) (int, error) {
	path := dailyPath(dir, day)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("journal: 打开 %s 失败: %w", path, err)
	}
	defer file.Close()
	maxSeq := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var header struct {
			TurnID string `json:"turn_id"`
		}
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			continue
		}
		if seq := sequenceOf(header.TurnID, day); seq > maxSeq {
			maxSeq = seq
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("journal: 读取 %s 失败: %w", path, err)
	}
	return maxSeq, nil
}

func readFile(path string) ([]TurnRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("journal: 打开 %s 失败: %w", path, err)
	}
	defer file.Close()
	records := []TurnRecord{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record TurnRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("journal: %s:%d 解析失败: %w", path, lineNumber, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("journal: 读取 %s 失败: %w", path, err)
	}
	return records, nil
}

// sequenceOf extracts the sequence from evt_<yyyymmdd>_<seq> for the given day.
func sequenceOf(turnID string, day time.Time) int {
	prefix := "evt_" + day.Format("20060102") + "_"
	if !strings.HasPrefix(turnID, prefix) {
		return 0
	}
	seq, err := strconv.Atoi(strings.TrimPrefix(turnID, prefix))
	if err != nil || seq < 0 {
		return 0
	}
	return seq
}
