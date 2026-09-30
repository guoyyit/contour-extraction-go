package contour

import "sort"

// Service 支持增量级别提取与确定性复现。
//
// 每个级别独立提取并缓存：AddLevels 只计算之前不存在的级别，
// 已缓存级别永不重算；全部结果按级别、再按折线规范序输出，
// 因此“一次性给全级别”和“分多次增量加入”的最终结果逐位一致。
type Service struct {
	grid *Grid
	cfg  Config

	mu     chan struct{}
	order  []float64
	cached map[float64][]Polyline
}

// NewService 在给定格网上创建服务。提取策略全部由 cfg 决定。
func NewService(g *Grid, cfg Config) *Service {
	nc, err := cfg.normalized()
	if err != nil {
		panic("contour: invalid config: " + err.Error())
	}
	return &Service{
		grid:   g,
		cfg:    nc,
		mu:     make(chan struct{}, 1),
		cached: make(map[float64][]Polyline),
	}
}

// AddLevels 增量加入级别；只提取新级别，已有级别保持不变。
// 返回值为本次实际新计算的级别（按升序）。
func (s *Service) AddLevels(levels ...float64) []float64 {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	added := make(map[float64]bool)
	for _, l := range levels {
		if _, ok := s.cached[l]; ok {
			continue
		}
		s.cached[l] = ExtractLevelTiledMust(s.grid, l, s.cfg)
		s.order = append(s.order, l)
		added[l] = true
	}
	out := make([]float64, 0, len(added))
	for l := range added {
		out = append(out, l)
	}
	sort.Float64s(out)
	return out
}

// Levels 返回当前已提取的全部级别（升序）。
func (s *Service) Levels() []float64 {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	out := append([]float64(nil), s.order...)
	sort.Float64s(out)
	return out
}

// Contours 返回某个级别已缓存的等值线（副本切片，避免外部修改缓存）。
func (s *Service) Contours(level float64) []Polyline {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	v, ok := s.cached[level]
	if !ok {
		return nil
	}
	out := make([]Polyline, len(v))
	copy(out, v)
	return out
}

// All 返回全部级别、按级别升序、级别内按规范序排列的等值线。
func (s *Service) All() []Polyline {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	levels := append([]float64(nil), s.order...)
	sort.Float64s(levels)
	var all []Polyline
	for _, l := range levels {
		all = append(all, s.cached[l]...)
	}
	return all
}

// ResultSnapshot 是可序列化、可复现的一次结果快照。
type ResultSnapshot struct {
	Config Config     `json:"config"`
	Levels []float64  `json:"levels"`
	Lines  []Polyline `json:"lines"`
}

// Snapshot 返回当前结果的可复现快照。
func (s *Service) Snapshot() *ResultSnapshot {
	return &ResultSnapshot{
		Config: s.cfg,
		Levels: s.Levels(),
		Lines:  s.All(),
	}
}

// ExtractLevelTiledMust 是服务内部使用的分块提取入口（配置已校验）。
func ExtractLevelTiledMust(g *Grid, level float64, cfg Config) []Polyline {
	lines, _ := ExtractLevelTiled(g, level, cfg)
	return lines
}
