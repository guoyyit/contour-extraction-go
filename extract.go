package contour

// ExtractLevel 在整个格网上以单块方式提取一个级别的等值线。
// 输出经过规范化排序，相同 (数据, 级别, 配置) 永远逐位一致。
func ExtractLevel(g *Grid, level float64, cfg Config) []Polyline {
	nc, err := cfg.normalized()
	if err != nil {
		panic("contour: invalid config: " + err.Error())
	}
	segs := extractSegments(g, level, CellRange{0, 0, g.CellRows(), g.CellCols()}, &nc)
	lines := stitch(level, segs, g, &nc)
	if nc.Simplify.Enabled {
		nc.simplifyGrid = g
		nc.simplifyLevel = level
		lines = simplifyPolylines(lines, nc)
	}
	return lines
}
