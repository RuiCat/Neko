// npz.go reads and writes the numpy artifacts the emote pipeline shares with
// Python: a compressed .npz archive holding X/centers float32 matrices and an
// int64 labels vector. Only the subset of the .npy format numpy needs for these
// arrays is supported (C order, little endian, numeric dtypes).
package emote

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// npArray is one decoded .npy member.
type npArray struct {
	descr string
	shape []int
	data  []byte
}

// vectorDB is the decoded vec_db.npz contents.
type vectorDB struct {
	X        []float32
	Centers  []float32
	Labels   []int64
	Dim      int
	Rows     int
	Clusters int
}

// npyMagic is the 8-byte .npy prefix (magic + format version 1.0).
var npyMagic = []byte{0x93, 'N', 'U', 'M', 'P', 'Y', 1, 0}

// readNPZ opens an npz archive and decodes the requested array names.
func readNPZ(path string, want ...string) (map[string]npArray, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("打开 npz 失败: %w", err)
	}
	defer reader.Close()
	wanted := map[string]bool{}
	for _, name := range want {
		wanted[name] = true
	}
	arrays := map[string]npArray{}
	for _, file := range reader.File {
		if !strings.HasSuffix(file.Name, ".npy") {
			continue
		}
		name := strings.TrimSuffix(file.Name, ".npy")
		if len(wanted) > 0 && !wanted[name] {
			continue
		}
		member, err := readZipMember(file)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", file.Name, err)
		}
		array, err := decodeNpy(member)
		if err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", file.Name, err)
		}
		arrays[name] = array
	}
	return arrays, nil
}

// readZipMember reads one archive member with a size guard.
func readZipMember(file *zip.File) ([]byte, error) {
	handle, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	// npz members are arrays; a 2 GiB cap keeps a corrupt file from exhausting
	// memory while allowing the full 6846x768 matrix.
	const maxMemberBytes = 2 << 30
	limited := io.LimitReader(handle, maxMemberBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxMemberBytes {
		return nil, fmt.Errorf("成员超过 %d 字节上限", maxMemberBytes)
	}
	return raw, nil
}

// decodeNpy parses one .npy payload.
func decodeNpy(raw []byte) (npArray, error) {
	if len(raw) < 10 || string(raw[:6]) != string(npyMagic[:6]) {
		return npArray{}, fmt.Errorf(".npy 魔数不匹配")
	}
	major := raw[6]
	var headerLen, offset int
	switch major {
	case 1:
		headerLen = int(binary.LittleEndian.Uint16(raw[8:10]))
		offset = 10
	case 2, 3:
		if len(raw) < 12 {
			return npArray{}, fmt.Errorf(".npy 头部截断")
		}
		headerLen = int(binary.LittleEndian.Uint32(raw[8:12]))
		offset = 12
	default:
		return npArray{}, fmt.Errorf("不支持的 .npy 版本 %d", major)
	}
	if len(raw) < offset+headerLen {
		return npArray{}, fmt.Errorf(".npy 头部截断")
	}
	header := string(raw[offset : offset+headerLen])
	descr, shape, fortran, err := parseNpyHeader(header)
	if err != nil {
		return npArray{}, err
	}
	if fortran {
		return npArray{}, fmt.Errorf("不支持 fortran_order")
	}
	data := raw[offset+headerLen:]
	return npArray{descr: descr, shape: shape, data: data}, nil
}

// parseNpyHeader extracts descr, shape and fortran_order from the header dict.
func parseNpyHeader(header string) (string, []int, bool, error) {
	descr, err := headerQuoted(header, "'descr':")
	if err != nil {
		return "", nil, false, err
	}
	shape, err := headerShape(header)
	if err != nil {
		return "", nil, false, err
	}
	fortran := headerBool(header, "'fortran_order':")
	return descr, shape, fortran, nil
}

// headerQuoted returns the quoted value that follows key in the header dict.
func headerQuoted(header, key string) (string, error) {
	index := strings.Index(header, key)
	if index < 0 {
		return "", fmt.Errorf(".npy 头部缺少 %s", key)
	}
	rest := header[index+len(key):]
	start := strings.Index(rest, "'")
	if start < 0 {
		return "", fmt.Errorf(".npy 头部 %s 非法", key)
	}
	rest = rest[start+1:]
	end := strings.Index(rest, "'")
	if end < 0 {
		return "", fmt.Errorf(".npy 头部 %s 非法", key)
	}
	return rest[:end], nil
}

// headerShape parses "'shape': (a, b)" including the 1-D trailing comma.
func headerShape(header string) ([]int, error) {
	index := strings.Index(header, "'shape':")
	if index < 0 {
		return nil, fmt.Errorf(".npy 头部缺少 shape")
	}
	rest := header[index+len("'shape':"):]
	open := strings.Index(rest, "(")
	closed := strings.Index(rest, ")")
	if open < 0 || closed < 0 || closed < open {
		return nil, fmt.Errorf(".npy 头部 shape 非法")
	}
	inner := strings.TrimSpace(rest[open+1 : closed])
	if inner == "" {
		return nil, nil
	}
	parts := strings.Split(inner, ",")
	shape := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf(".npy 头部 shape 非法: %v", err)
		}
		shape = append(shape, value)
	}
	return shape, nil
}

// headerBool reports whether key's value is True.
func headerBool(header, key string) bool {
	index := strings.Index(header, key)
	if index < 0 {
		return false
	}
	rest := strings.TrimSpace(header[index+len(key):])
	return strings.HasPrefix(rest, "True")
}

// descriptorSize maps a numpy descriptor to its element size in bytes.
func descriptorSize(descr string) (int, error) {
	switch descr {
	case "<f4", "<i4", "<u4":
		return 4, nil
	case "<f8", "<i8", "<u8":
		return 8, nil
	case "<i2", "<u2":
		return 2, nil
	case "|i1", "|u1", "|b1":
		return 1, nil
	}
	return 0, fmt.Errorf("不支持的向量类型 %q", descr)
}

// floats converts the array to float32 values (float64 rows are narrowed).
func (array npArray) floats() ([]float32, error) {
	size, err := descriptorSize(array.descr)
	if err != nil {
		return nil, err
	}
	count, err := elementCount(array.shape)
	if err != nil {
		return nil, err
	}
	if len(array.data) != count*size {
		return nil, fmt.Errorf("数据长度 %d 与 %d 个元素不符", len(array.data), count)
	}
	values := make([]float32, count)
	switch array.descr {
	case "<f4":
		for i := range values {
			values[i] = math.Float32frombits(binary.LittleEndian.Uint32(array.data[i*4:]))
		}
	case "<f8":
		for i := range values {
			values[i] = float32(math.Float64frombits(binary.LittleEndian.Uint64(array.data[i*8:])))
		}
	default:
		return nil, fmt.Errorf("数组 %q 不是浮点类型", array.descr)
	}
	return values, nil
}

// int64s converts the array to int64 values.
func (array npArray) int64s() ([]int64, error) {
	size, err := descriptorSize(array.descr)
	if err != nil {
		return nil, err
	}
	count, err := elementCount(array.shape)
	if err != nil {
		return nil, err
	}
	if len(array.data) != count*size {
		return nil, fmt.Errorf("数据长度 %d 与 %d 个元素不符", len(array.data), count)
	}
	values := make([]int64, count)
	switch array.descr {
	case "<i8":
		for i := range values {
			values[i] = int64(binary.LittleEndian.Uint64(array.data[i*8:]))
		}
	case "<i4":
		for i := range values {
			values[i] = int64(int32(binary.LittleEndian.Uint32(array.data[i*4:])))
		}
	case "<i2":
		for i := range values {
			values[i] = int64(int16(binary.LittleEndian.Uint16(array.data[i*2:])))
		}
	default:
		return nil, fmt.Errorf("标签数组类型 %q 不受支持", array.descr)
	}
	return values, nil
}

// elementCount multiplies the shape (0 for an empty shape).
func elementCount(shape []int) (int, error) {
	count := 1
	for _, dim := range shape {
		if dim < 0 {
			return 0, fmt.Errorf("负维度")
		}
		count *= dim
	}
	if len(shape) == 0 {
		return 0, nil
	}
	return count, nil
}

// readVecDB decodes vec_db.npz into the typed database.
func readVecDB(path string) (*vectorDB, error) {
	arrays, err := readNPZ(path, "X", "centers", "labels")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVectorDBCorrupt, err)
	}
	rawX, ok := arrays["X"]
	if !ok {
		return nil, fmt.Errorf("%w: 缺少 X", ErrVectorDBCorrupt)
	}
	rawCenters, ok := arrays["centers"]
	if !ok {
		return nil, fmt.Errorf("%w: 缺少 centers", ErrVectorDBCorrupt)
	}
	rawLabels, ok := arrays["labels"]
	if !ok {
		return nil, fmt.Errorf("%w: 缺少 labels", ErrVectorDBCorrupt)
	}
	x, err := rawX.floats()
	if err != nil {
		return nil, fmt.Errorf("%w: X: %v", ErrVectorDBCorrupt, err)
	}
	centers, err := rawCenters.floats()
	if err != nil {
		return nil, fmt.Errorf("%w: centers: %v", ErrVectorDBCorrupt, err)
	}
	labels, err := rawLabels.int64s()
	if err != nil {
		return nil, fmt.Errorf("%w: labels: %v", ErrVectorDBCorrupt, err)
	}
	rows := len(labels)
	if len(rawX.shape) != 2 || len(rawCenters.shape) != 2 {
		return nil, fmt.Errorf("%w: 数组维度不是二维", ErrVectorDBCorrupt)
	}
	dim := rawX.shape[1]
	if rawCenters.shape[1] != dim {
		return nil, fmt.Errorf("%w: X 与 centers 维度不同", ErrVectorDimension)
	}
	clusters := rawCenters.shape[0]
	if rawX.shape[0] != rows {
		return nil, fmt.Errorf("%w: X 行数与 labels 不符", ErrVectorDBCorrupt)
	}
	if len(x) != rows*dim || len(centers) != clusters*dim {
		return nil, fmt.Errorf("%w: 数据长度与形状不符", ErrVectorDBCorrupt)
	}
	return &vectorDB{X: x, Centers: centers, Labels: labels, Dim: dim, Rows: rows, Clusters: clusters}, nil
}

// writeVecDB writes a numpy-compatible vec_db.npz (Python can np.load it).
func writeVecDB(path string, x, centers []float32, labels []int64, rows, dim, clusters int) error {
	if len(x) != rows*dim || len(centers) != clusters*dim || len(labels) != rows {
		return fmt.Errorf("向量库形状不一致: x=%d labels=%d rows=%d dim=%d clusters=%d", len(x), len(labels), rows, dim, clusters)
	}
	handle, err := os.Create(path)
	if err != nil {
		return err
	}
	defer handle.Close()
	writer := zip.NewWriter(handle)
	if err := writeNpyMember(writer, "X.npy", "<f4", []int{rows, dim}, float32Bytes(x)); err != nil {
		return err
	}
	if err := writeNpyMember(writer, "centers.npy", "<f4", []int{clusters, dim}, float32Bytes(centers)); err != nil {
		return err
	}
	if err := writeNpyMember(writer, "labels.npy", "<i8", []int{rows}, int64Bytes(labels)); err != nil {
		return err
	}
	return writer.Close()
}

// writeNpyMember appends one deflated .npy member to the archive.
func writeNpyMember(writer *zip.Writer, name, descr string, shape []int, data []byte) error {
	member, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = member.Write(encodeNpy(descr, shape, data))
	return err
}

// encodeNpy builds a .npy payload (format 1.0) with a 64-byte aligned header.
func encodeNpy(descr string, shape []int, data []byte) []byte {
	dict := fmt.Sprintf("{'descr': '%s', 'fortran_order': False, 'shape': %s, }", descr, shapeLiteral(shape))
	padding := (64 - (len(npyMagic)+2+len(dict)+1)%64) % 64
	header := dict + strings.Repeat(" ", padding) + "\n"
	payload := make([]byte, 0, len(npyMagic)+2+len(header)+len(data))
	payload = append(payload, npyMagic...)
	var length [2]byte
	binary.LittleEndian.PutUint16(length[:], uint16(len(header)))
	payload = append(payload, length[:]...)
	payload = append(payload, header...)
	payload = append(payload, data...)
	return payload
}

// shapeLiteral renders a numpy shape tuple.
func shapeLiteral(shape []int) string {
	if len(shape) == 0 {
		return "()"
	}
	parts := make([]string, len(shape))
	for i, dim := range shape {
		parts[i] = strconv.Itoa(dim)
	}
	if len(shape) == 1 {
		return "(" + parts[0] + ",)"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// float32Bytes encodes values little-endian.
func float32Bytes(values []float32) []byte {
	data := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	return data
}

// int64Bytes encodes values little-endian.
func int64Bytes(values []int64) []byte {
	data := make([]byte, len(values)*8)
	for i, value := range values {
		binary.LittleEndian.PutUint64(data[i*8:], uint64(value))
	}
	return data
}
