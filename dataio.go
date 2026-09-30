package contour

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// ReadGridFile 读取本地空格分隔的规则格网文本文件：
//
//	rows cols
//	v00 v01 ...
//	...
//
// 数值支持普通浮点文本；字面量 "nan"（大小写不敏感）表示未定义格点。
// 全部数据在本地读取，不访问任何外部服务。
func ReadGridFile(path string) (*Grid, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)

	header, err := r.ReadString('\n')
	if err != nil {
		return nil, errors.New("grid file: missing header")
	}
	fields := strings.Fields(header)
	if len(fields) != 2 {
		return nil, errors.New("grid file: header must be 'rows cols'")
	}
	rows, err1 := strconv.Atoi(fields[0])
	cols, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil || rows < 2 || cols < 2 {
		return nil, errors.New("grid file: invalid dimensions")
	}

	vals := make([]float64, 0, rows*cols)
	for rr := 0; rr < rows; rr++ {
		line, err := r.ReadString('\n')
		if err != nil && rr < rows-1 {
			return nil, fmt.Errorf("grid file: row %d unreadable: %w", rr, err)
		}
		rowFields := strings.Fields(line)
		if len(rowFields) != cols {
			return nil, fmt.Errorf("grid file: row %d has %d values, want %d", rr, len(rowFields), cols)
		}
		for _, tok := range rowFields {
			v, err := parseGridToken(tok)
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}
	}
	return NewGrid(rows, cols, vals)
}

func parseGridToken(tok string) (float64, error) {
	if strings.EqualFold(tok, "nan") {
		return math.NaN(), nil
	}
	v, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, fmt.Errorf("grid file: invalid number %q", tok)
	}
	return v, nil
}

// WriteGridFile 以与 ReadGridFile 相同的文本格式写回格网。
func WriteGridFile(path string, g *Grid) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := fmt.Fprintf(w, "%d %d\n", g.rows, g.cols); err != nil {
		return err
	}
	for r := 0; r < g.rows; r++ {
		for c := 0; c < g.cols; c++ {
			if c > 0 {
				if _, err := w.WriteString(" "); err != nil {
					return err
				}
			}
			v := g.Value(r, c)
			tok := strconv.FormatFloat(v, 'g', -1, 64)
			if _, err := w.WriteString(tok); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}

// WriteContoursJSON 把可复现快照序列化为本地 JSON 文件。
func WriteContoursJSON(path string, snap *ResultSnapshot) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

// ReadContoursJSON 读回 WriteContoursJSON 写出的快照。
func ReadContoursJSON(path string) (*ResultSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var snap ResultSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
