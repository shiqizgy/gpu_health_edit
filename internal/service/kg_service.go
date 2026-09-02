package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/logger"
)

// ============================================================================
// 故障知识图谱服务层
//
// 职责：输入校验、事务编排、有界图遍历、幂等导入。
// 与评分链路零耦合——本文件不 import 任何 scoring 包，也不读写评分相关的表。
// ============================================================================

// 哨兵错误。handler 层按这些错误决定 HTTP 状态码，
// 避免把 error.Error() 直接透给前端造成信息泄露或状态码混乱。
var (
	ErrKGNotFound   = errors.New("节点或关系不存在")
	ErrKGConflict   = errors.New("数据已被他人修改，请刷新后重试")
	ErrKGDuplicate  = errors.New("已存在相同的节点或关系")
	ErrKGValidation = errors.New("参数校验失败")
)

// 遍历与返回规模的硬上限。
// 图谱页面一次画超过几百个节点就没有可读性了，而且力导向布局会卡；
// 这些上限同时也是防止恶意/误用参数打爆内存的保护。
const (
	MaxGraphNodes   = 1000 // 单次返回节点数上限
	DefaultGraphCap = 300  // 未指定 limit 时的默认值
	MaxExpandDepth  = 3    // 邻域展开最大跳数
)

// KGService 知识图谱服务。
type KGService struct {
	repo   *repository.KGRepo
	metric *repository.MetricRepo // 仅用于校验指标节点引用是否有效（只读）
	fault  *repository.FaultRepo  // 仅用于从既有故障知识条目导入（只读）
}

func NewKGService(
	repo *repository.KGRepo,
	metric *repository.MetricRepo,
	fault *repository.FaultRepo,
) *KGService {
	return &KGService{repo: repo, metric: metric, fault: fault}
}

// ---------------------------------------------------------------------------
// 对外 DTO
// ---------------------------------------------------------------------------

// GraphDTO 一次性返回给前端的子图。
type GraphDTO struct {
	Nodes     []model.KGNode `json:"nodes"`
	Edges     []model.KGEdge `json:"edges"`
	Truncated bool           `json:"truncated"` // 是否因为超过上限被截断
}

// MetaDTO 图谱元数据：类型、关系、配色、统计。
type MetaDTO struct {
	NodeTypes  []model.NodeTypeMeta `json:"node_types"`
	RelTypes   []model.RelTypeMeta  `json:"rel_types"`
	NodeCounts map[string]int64     `json:"node_counts"`
	EdgeCount  int64                `json:"edge_count"`
}

// NodeDetailDTO 节点详情 + 它的全部关系（含对端节点信息，前端直接渲染）。
type NodeDetailDTO struct {
	Node      model.KGNode   `json:"node"`
	Edges     []EdgeWithPeer `json:"edges"`
	MetricRef *MetricRefDTO  `json:"metric_ref,omitempty"` // 指标节点的引用校验结果
}

// EdgeWithPeer 一条边 + 对端节点的摘要信息。
type EdgeWithPeer struct {
	Edge      model.KGEdge `json:"edge"`
	Direction string       `json:"direction"` // out=本节点为起点，in=本节点为终点
	PeerID    uint64       `json:"peer_id"`
	PeerName  string       `json:"peer_name"`
	PeerType  string       `json:"peer_type"`
}

// MetricRefDTO 指标节点弱引用的校验结果。
// 指标定义被改名或删除时，图谱不会报错，但这里会标记为未匹配，
// 提示维护者去修正——这是"弱引用"设计的配套可观测手段。
type MetricRefDTO struct {
	MetricName string `json:"metric_name"`
	Exists     bool   `json:"exists"`
	Dimension  string `json:"dimension,omitempty"`
	CardType   string `json:"card_type,omitempty"`
}

// ---------------------------------------------------------------------------
// 元数据
// ---------------------------------------------------------------------------

// Meta 返回类型定义与规模统计。
func (s *KGService) Meta() (*MetaDTO, error) {
	counts, err := s.repo.CountNodesByType()
	if err != nil {
		return nil, err
	}
	edgeCnt, err := s.repo.CountEdges()
	if err != nil {
		return nil, err
	}
	return &MetaDTO{
		NodeTypes:  model.NodeTypes,
		RelTypes:   model.RelTypes,
		NodeCounts: counts,
		EdgeCount:  edgeCnt,
	}, nil
}

// ---------------------------------------------------------------------------
// 子图查询
// ---------------------------------------------------------------------------

// GraphOptions 子图查询参数。
type GraphOptions struct {
	NodeType string
	Severity string
	Keyword  string
	Limit    int
}

// Graph 按筛选条件返回一个子图。
//
// 语义：先按条件筛出节点（受 limit 限制），再取「两端都在结果集内」的边。
// 这样保证返回的图是自洽的——不会出现边指向画布外的节点。
func (s *KGService) Graph(opt GraphOptions) (*GraphDTO, error) {
	limit := normalizeCap(opt.Limit)

	// 多取一条用于判断是否被截断
	nodes, total, err := s.repo.ListNodes(repository.NodeQuery{
		NodeType: opt.NodeType,
		Severity: opt.Severity,
		Keyword:  opt.Keyword,
		Limit:    limit,
		Offset:   0,
	})
	if err != nil {
		return nil, err
	}

	ids := make([]uint64, 0, len(nodes))
	for i := range nodes {
		ids = append(ids, nodes[i].ID)
	}
	edges, err := s.repo.ListEdgesWithin(ids)
	if err != nil {
		return nil, err
	}

	return &GraphDTO{
		Nodes:     nodes,
		Edges:     edges,
		Truncated: total > int64(len(nodes)),
	}, nil
}

// Neighbors 以 rootID 为中心做有界 BFS 展开。
//
// 三重保护：
//  1. depth 上限 MaxExpandDepth，防止无界遍历
//  2. 节点总数上限 limit，达到即停止扩展并置 Truncated
//  3. visited 集合去重，天然容忍图中的环（CAUSES 链可能成环）
//
// 每一跳只发两条 SQL（查边 + 查新节点），跳数固定，不存在 N+1。
func (s *KGService) Neighbors(rootID uint64, depth, limit int) (*GraphDTO, error) {
	if depth <= 0 {
		depth = 1
	}
	if depth > MaxExpandDepth {
		depth = MaxExpandDepth
	}
	cap_ := normalizeCap(limit)

	root, err := s.repo.GetNode(rootID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKGNotFound
		}
		return nil, err
	}

	visited := map[uint64]struct{}{rootID: {}}
	frontier := []uint64{rootID}
	edgeSet := map[uint64]model.KGEdge{}
	truncated := false

	for hop := 0; hop < depth && len(frontier) > 0 && !truncated; hop++ {
		edges, err := s.repo.ListEdgesTouching(frontier)
		if err != nil {
			return nil, err
		}

		// 收集本跳新发现的节点。排序保证同样的输入得到同样的截断结果，
		// 避免前端反复请求时图形一直抖动。
		newIDs := make([]uint64, 0)
		for _, e := range edges {
			edgeSet[e.ID] = e
			for _, peer := range []uint64{e.FromID, e.ToID} {
				if _, ok := visited[peer]; ok {
					continue
				}
				visited[peer] = struct{}{}
				newIDs = append(newIDs, peer)
			}
		}
		sort.Slice(newIDs, func(i, j int) bool { return newIDs[i] < newIDs[j] })

		if len(visited) > cap_ {
			// 超限：回退掉多出来的节点，本跳到此为止
			over := len(visited) - cap_
			for i := len(newIDs) - 1; i >= 0 && over > 0; i-- {
				delete(visited, newIDs[i])
				newIDs = newIDs[:i]
				over--
			}
			truncated = true
		}
		frontier = newIDs
	}

	ids := make([]uint64, 0, len(visited))
	for id := range visited {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	nodes, err := s.repo.GetNodesByIDs(ids)
	if err != nil {
		return nil, err
	}

	// 只保留两端都在结果集里的边，防止悬空引用
	inSet := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		inSet[id] = struct{}{}
	}
	edges := make([]model.KGEdge, 0, len(edgeSet))
	for _, e := range edgeSet {
		if _, okF := inSet[e.FromID]; !okF {
			continue
		}
		if _, okT := inSet[e.ToID]; !okT {
			continue
		}
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	_ = root // root 已包含在 nodes 中，此处仅用于前置存在性校验
	return &GraphDTO{Nodes: nodes, Edges: edges, Truncated: truncated}, nil
}

// ListNodesPaged 分页节点列表，供表格视图使用。
// 与 Graph 分开是因为表格需要真实的总数和翻页，而图视图只需要一个有界快照。
func (s *KGService) ListNodesPaged(
	nodeType, severity, keyword string, limit, offset int,
) ([]model.KGNode, int64, error) {
	return s.repo.ListNodes(repository.NodeQuery{
		NodeType: nodeType, Severity: severity, Keyword: keyword,
		Limit: limit, Offset: offset,
	})
}

// NodeDetail 返回节点详情及其全部关系（带对端信息）。
func (s *KGService) NodeDetail(id uint64) (*NodeDetailDTO, error) {
	node, err := s.repo.GetNode(id)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKGNotFound
		}
		return nil, err
	}

	edges, err := s.repo.ListEdgesOfNode(id)
	if err != nil {
		return nil, err
	}

	// 一次性把所有对端节点查出来，避免逐条边查询
	peerIDs := make([]uint64, 0, len(edges))
	for _, e := range edges {
		if e.FromID == id {
			peerIDs = append(peerIDs, e.ToID)
		} else {
			peerIDs = append(peerIDs, e.FromID)
		}
	}
	peers, err := s.repo.GetNodesByIDs(dedupIDs(peerIDs))
	if err != nil {
		return nil, err
	}
	peerMap := make(map[uint64]model.KGNode, len(peers))
	for _, p := range peers {
		peerMap[p.ID] = p
	}

	out := make([]EdgeWithPeer, 0, len(edges))
	for _, e := range edges {
		dir, peerID := "out", e.ToID
		if e.ToID == id {
			dir, peerID = "in", e.FromID
		}
		p := peerMap[peerID]
		out = append(out, EdgeWithPeer{
			Edge: e, Direction: dir,
			PeerID: peerID, PeerName: p.Name, PeerType: p.NodeType,
		})
	}

	detail := &NodeDetailDTO{Node: *node, Edges: out}
	if node.NodeType == model.NodeTypeMetric {
		detail.MetricRef = s.checkMetricRef(node)
	}
	return detail, nil
}

// checkMetricRef 校验指标节点的弱引用是否还能在指标定义表中找到。
// 查不到不是错误，只是标记为未匹配——评分链路不受任何影响。
func (s *KGService) checkMetricRef(n *model.KGNode) *MetricRefDTO {
	name := propString(n.Props, "metric_name")
	if name == "" {
		name = n.Name // 兼容直接用指标名当节点名的情况
	}
	ref := &MetricRefDTO{MetricName: name}
	if name == "" || s.metric == nil {
		return ref
	}
	defs, err := s.metric.AllDefsMap()
	if err != nil {
		logger.L.Warnf("知识图谱：校验指标引用失败 metric=%s: %v", name, err)
		return ref
	}
	if d, ok := defs[name]; ok {
		ref.Exists = true
		ref.Dimension = d.Dimension
		ref.CardType = d.CardType
	}
	return ref
}

// ---------------------------------------------------------------------------
// 节点写操作
// ---------------------------------------------------------------------------

// NodeInput 创建/更新节点的入参。
type NodeInput struct {
	NodeKey     string `json:"node_key"`
	NodeType    string `json:"node_type"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Props       string `json:"props"`
	Version     int    `json:"version"` // 更新时必填，用于乐观锁
}

var validSeverity = map[string]bool{"": true, "warning": true, "critical": true, "fatal": true}

// validateNode 集中校验。所有长度上限都比 DDL 的列宽小一档，
// 让错误在应用层被拦住并给出可读提示，而不是抛出一个 MySQL 截断错误。
func (s *KGService) validateNode(in *NodeInput) error {
	in.NodeType = strings.TrimSpace(in.NodeType)
	in.Name = strings.TrimSpace(in.Name)
	in.NodeKey = strings.TrimSpace(in.NodeKey)
	in.Severity = strings.TrimSpace(in.Severity)

	if !model.IsValidNodeType(in.NodeType) {
		return fmt.Errorf("%w: 未知的节点类型 %q", ErrKGValidation, in.NodeType)
	}
	if in.Name == "" {
		return fmt.Errorf("%w: 节点名称不能为空", ErrKGValidation)
	}
	if len([]rune(in.Name)) > 100 {
		return fmt.Errorf("%w: 节点名称不能超过 100 字", ErrKGValidation)
	}
	if len([]rune(in.Summary)) > 200 {
		return fmt.Errorf("%w: 摘要不能超过 200 字", ErrKGValidation)
	}
	if !validSeverity[in.Severity] {
		return fmt.Errorf("%w: 严重等级只能是 warning / critical / fatal", ErrKGValidation)
	}
	if in.NodeType != model.NodeTypeFault && in.Severity != "" {
		return fmt.Errorf("%w: 只有故障节点才能设置严重等级", ErrKGValidation)
	}
	if err := validateJSONObject(in.Props); err != nil {
		return fmt.Errorf("%w: 扩展属性 %v", ErrKGValidation, err)
	}
	if in.NodeKey != "" && len(in.NodeKey) > 180 {
		return fmt.Errorf("%w: node_key 不能超过 180 字符", ErrKGValidation)
	}
	return nil
}

// CreateNode 新建节点。node_key 留空时按 类型:名称 自动生成。
func (s *KGService) CreateNode(in *NodeInput) (*model.KGNode, error) {
	if err := s.validateNode(in); err != nil {
		return nil, err
	}
	if in.NodeKey == "" {
		in.NodeKey = buildNodeKey(in.NodeType, in.Name)
	}
	if in.Props == "" {
		in.Props = "{}"
	}

	n := &model.KGNode{
		NodeKey: in.NodeKey, NodeType: in.NodeType, Name: in.Name,
		Summary: in.Summary, Description: in.Description,
		Severity: in.Severity, Props: in.Props,
	}
	if err := s.repo.CreateNode(n); err != nil {
		if repository.IsDuplicate(err) {
			return nil, fmt.Errorf("%w: node_key=%s 已存在", ErrKGDuplicate, in.NodeKey)
		}
		return nil, err
	}
	logger.L.Infof("知识图谱：新建节点 id=%d key=%s type=%s", n.ID, n.NodeKey, n.NodeType)
	return n, nil
}

// UpdateNode 更新节点。node_type 不可变——类型变了会让已有的边违反端点约束，
// 需要改类型时请删除后重建。
func (s *KGService) UpdateNode(id uint64, in *NodeInput) (*model.KGNode, error) {
	old, err := s.repo.GetNode(id)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKGNotFound
		}
		return nil, err
	}
	if in.Version <= 0 {
		return nil, fmt.Errorf("%w: 缺少 version 字段", ErrKGValidation)
	}
	// 强制沿用原类型后再校验，避免调用方漏传 node_type 时被判成非法类型
	in.NodeType = old.NodeType
	if err := s.validateNode(in); err != nil {
		return nil, err
	}
	if in.Props == "" {
		in.Props = "{}"
	}

	fields := map[string]any{
		"name":        in.Name,
		"summary":     in.Summary,
		"description": in.Description,
		"severity":    in.Severity,
		"props":       in.Props,
	}
	if in.NodeKey != "" && in.NodeKey != old.NodeKey {
		fields["node_key"] = in.NodeKey
	}

	rows, err := s.repo.UpdateNode(id, in.Version, fields)
	if err != nil {
		if repository.IsDuplicate(err) {
			return nil, fmt.Errorf("%w: node_key 已被其他节点占用", ErrKGDuplicate)
		}
		return nil, err
	}
	if rows == 0 {
		return nil, ErrKGConflict
	}
	logger.L.Infof("知识图谱：更新节点 id=%d", id)
	return s.repo.GetNode(id)
}

// DeleteNode 删除节点及其全部关联边，返回被连带删除的边数。
func (s *KGService) DeleteNode(id uint64) (int64, error) {
	n, err := s.repo.DeleteNodeCascade(id)
	if err != nil {
		if repository.IsNotFound(err) {
			return 0, ErrKGNotFound
		}
		return 0, err
	}
	logger.L.Infof("知识图谱：删除节点 id=%d，连带删除 %d 条关系", id, n)
	return n, nil
}

// ---------------------------------------------------------------------------
// 边写操作
// ---------------------------------------------------------------------------

// EdgeInput 创建/更新边的入参。
type EdgeInput struct {
	FromID  uint64  `json:"from_id"`
	ToID    uint64  `json:"to_id"`
	RelType string  `json:"rel_type"`
	Label   string  `json:"label"`
	Weight  float64 `json:"weight"`
	Props   string  `json:"props"`
	Version int     `json:"version"`
}

// CreateEdge 新建关系。
//
// 校验分两段：
//   - 不依赖节点的部分（自环、关系类型、权重、JSON）在进事务前做完，快速失败
//   - 依赖节点类型的端点约束在事务内、拿到加锁的节点之后做
func (s *KGService) CreateEdge(in *EdgeInput) (*model.KGEdge, error) {
	in.RelType = strings.TrimSpace(in.RelType)

	if in.FromID == 0 || in.ToID == 0 {
		return nil, fmt.Errorf("%w: 起点和终点都必须指定", ErrKGValidation)
	}
	if in.FromID == in.ToID {
		return nil, fmt.Errorf("%w: 不允许节点连向自己", ErrKGValidation)
	}
	if !model.IsValidRelType(in.RelType) {
		return nil, fmt.Errorf("%w: 未知的关系类型 %q", ErrKGValidation, in.RelType)
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	if in.Weight > 1 {
		return nil, fmt.Errorf("%w: 关联强度取值范围为 0~1", ErrKGValidation)
	}
	if len([]rune(in.Label)) > 60 {
		return nil, fmt.Errorf("%w: 关系说明不能超过 60 字", ErrKGValidation)
	}
	if err := validateJSONObject(in.Props); err != nil {
		return nil, fmt.Errorf("%w: 扩展属性 %v", ErrKGValidation, err)
	}
	if in.Props == "" {
		in.Props = "{}"
	}

	e := &model.KGEdge{
		FromID: in.FromID, ToID: in.ToID, RelType: in.RelType,
		Label: in.Label, Weight: in.Weight, Props: in.Props,
	}

	err := s.repo.CreateEdgeChecked(e, func(from, to *model.KGNode) error {
		if !model.AllowRelPair(in.RelType, from.NodeType, to.NodeType) {
			return fmt.Errorf("%w: 关系「%s」不允许从「%s」连到「%s」",
				ErrKGValidation, relLabel(in.RelType),
				nodeTypeLabel(from.NodeType), nodeTypeLabel(to.NodeType))
		}
		return nil
	})
	if err != nil {
		if repository.IsDuplicate(err) {
			return nil, fmt.Errorf("%w: 这两个节点之间已存在同类型关系", ErrKGDuplicate)
		}
		if repository.IsNotFound(err) {
			return nil, ErrKGNotFound
		}
		return nil, err
	}
	logger.L.Infof("知识图谱：新建关系 id=%d %d-[%s]->%d", e.ID, e.FromID, e.RelType, e.ToID)
	return e, nil
}

// UpdateEdge 更新关系的说明、强度和扩展属性。
// 端点与关系类型不可变：改了就等于是另一条边，唯一索引的约束会被绕开。
func (s *KGService) UpdateEdge(id uint64, in *EdgeInput) (*model.KGEdge, error) {
	if _, err := s.repo.GetEdge(id); err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKGNotFound
		}
		return nil, err
	}
	if in.Version <= 0 {
		return nil, fmt.Errorf("%w: 缺少 version 字段", ErrKGValidation)
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	if in.Weight > 1 {
		return nil, fmt.Errorf("%w: 关联强度取值范围为 0~1", ErrKGValidation)
	}
	if len([]rune(in.Label)) > 60 {
		return nil, fmt.Errorf("%w: 关系说明不能超过 60 字", ErrKGValidation)
	}
	if err := validateJSONObject(in.Props); err != nil {
		return nil, fmt.Errorf("%w: 扩展属性 %v", ErrKGValidation, err)
	}
	if in.Props == "" {
		in.Props = "{}"
	}

	rows, err := s.repo.UpdateEdge(id, in.Version, map[string]any{
		"label": in.Label, "weight": in.Weight, "props": in.Props,
	})
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrKGConflict
	}
	return s.repo.GetEdge(id)
}

// DeleteEdge 删除关系。
func (s *KGService) DeleteEdge(id uint64) error {
	rows, err := s.repo.DeleteEdge(id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrKGNotFound
	}
	logger.L.Infof("知识图谱：删除关系 id=%d", id)
	return nil
}

// ---------------------------------------------------------------------------
// 从既有故障知识条目导入
// ---------------------------------------------------------------------------

// ImportResult 导入结果统计。
type ImportResult struct {
	FaultCreated  int      `json:"fault_created"`
	MetricCreated int      `json:"metric_created"`
	EdgeCreated   int      `json:"edge_created"`
	Skipped       int      `json:"skipped"`
	Warnings      []string `json:"warnings"`
}

// ImportFromFaultKnowledge 把 fault_knowledge 表中的条目导入为图谱节点。
//
// 幂等设计：所有节点按 node_key 判重，已存在则跳过而不是覆盖。
// 「跳过」而非「更新」是刻意的——图谱内容会被人工编辑，
// 重跑导入不应该把人工修订冲掉。
//
// 只导入能可靠推导的部分：故障节点 + 指标节点 + INDICATED_BY 关系。
// 概念和影响范围没有对应的源字段，由维护者在页面上手工补充。
func (s *KGService) ImportFromFaultKnowledge() (*ImportResult, error) {
	res := &ImportResult{Warnings: []string{}}
	if s.fault == nil {
		return nil, fmt.Errorf("%w: 故障知识仓储未注入", ErrKGValidation)
	}

	items, _, err := s.fault.List("", "", 1000, 0)
	if err != nil {
		return nil, err
	}

	// 预取指标定义，用于校验 related_metrics 里的指标名是否真实存在
	defs, err := s.metric.AllDefsMap()
	if err != nil {
		return nil, err
	}

	for _, it := range items {
		faultKey := buildNodeKey(model.NodeTypeFault, it.FaultType)
		faultNode, err := s.repo.GetNodeByKey(faultKey)

		switch {
		case err == nil:
			res.Skipped++ // 已存在，保留人工编辑结果
		case repository.IsNotFound(err):
			props, _ := json.Marshal(map[string]string{
				"xid_code": it.XIDCode,
				"source":   "fault_knowledge",
			})
			faultNode = &model.KGNode{
				NodeKey:     faultKey,
				NodeType:    model.NodeTypeFault,
				Name:        it.FaultType,
				Summary:     truncateRunes(it.Symptom, 200),
				Description: composeFaultDesc(it.Symptom, it.PossibleCause, it.Suggestion),
				Severity:    normalizeSeverity(it.Severity),
				Props:       string(props),
			}
			if err := s.repo.CreateNode(faultNode); err != nil {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("创建故障节点「%s」失败：%v", it.FaultType, err))
				continue
			}
			res.FaultCreated++
		default:
			return nil, err
		}

		// related_metrics 存的是指标名 JSON 数组，解析失败不影响其他条目
		var metricNames []string
		if strings.TrimSpace(it.RelatedMetrics) != "" {
			if err := json.Unmarshal([]byte(it.RelatedMetrics), &metricNames); err != nil {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("故障「%s」的 related_metrics 不是合法 JSON 数组，已跳过关联指标", it.FaultType))
				metricNames = nil
			}
		}

		for _, mn := range metricNames {
			mn = strings.TrimSpace(mn)
			if mn == "" {
				continue
			}
			metricKey := buildNodeKey(model.NodeTypeMetric, mn)
			mNode, err := s.repo.GetNodeByKey(metricKey)
			if repository.IsNotFound(err) {
				props := map[string]string{"metric_name": mn}
				summary := ""
				if d, ok := defs[mn]; ok {
					props["card_type"] = d.CardType
					props["dimension"] = d.Dimension
					summary = truncateRunes(d.Conception, 200)
				} else {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("指标「%s」在指标定义表中不存在，已作为孤立节点创建", mn))
				}
				pj, _ := json.Marshal(props)
				mNode = &model.KGNode{
					NodeKey: metricKey, NodeType: model.NodeTypeMetric,
					Name: mn, Summary: summary, Props: string(pj),
				}
				if err := s.repo.CreateNode(mNode); err != nil {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("创建指标节点「%s」失败：%v", mn, err))
					continue
				}
				res.MetricCreated++
			} else if err != nil {
				return nil, err
			}

			edge := &model.KGEdge{
				FromID: faultNode.ID, ToID: mNode.ID,
				RelType: model.RelIndicatedBy, Weight: 1, Props: "{}",
			}
			if err := s.repo.CreateEdgeChecked(edge, nil); err != nil {
				if !repository.IsDuplicate(err) {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("建立「%s → %s」关系失败：%v", it.FaultType, mn, err))
				}
				continue
			}
			res.EdgeCreated++
		}
	}

	logger.L.Infof("知识图谱导入完成：新建故障 %d、指标 %d、关系 %d，跳过 %d",
		res.FaultCreated, res.MetricCreated, res.EdgeCreated, res.Skipped)
	return res, nil
}

// MetricOptions 返回指标名候选列表，供前端新建指标节点时下拉选择，
// 避免手输造成的拼写错误。只读 accel_metric_scoring，不做任何写操作。
func (s *KGService) MetricOptions(keyword string, limit int) ([]map[string]string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	defs, _, err := s.metric.List(repository.MetricQuery{
		Keyword: keyword, Limit: limit, Offset: 0,
	})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]string{
			"metric_name": d.MetricName,
			"card_type":   d.CardType,
			"dimension":   d.Dimension,
			"concept":     truncateRunes(d.Conception, 80),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 内部工具
// ---------------------------------------------------------------------------

func normalizeCap(n int) int {
	if n <= 0 {
		return DefaultGraphCap
	}
	if n > MaxGraphNodes {
		return MaxGraphNodes
	}
	return n
}

// validateJSONObject 校验字符串是合法的 JSON 对象（不是数组或标量）。
// 空串视为合法，由调用方补成 "{}"。
func validateJSONObject(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return errors.New("必须是合法的 JSON 对象，例如 {\"key\":\"value\"}")
	}
	return nil
}

// propString 从 props JSON 中安全取一个字符串字段，解析失败返回空串。
func propString(props, key string) string {
	if strings.TrimSpace(props) == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(props), &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// buildNodeKey 生成业务唯一键。名称中的空白统一压成下划线，
// 保证同一个概念不会因为多打了个空格产生两个节点。
func buildNodeKey(nodeType, name string) string {
	k := strings.Join(strings.Fields(name), "_")
	key := nodeType + ":" + k
	if len(key) > 180 {
		key = key[:180]
	}
	return key
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fatal":
		return "fatal"
	case "critical":
		return "critical"
	default:
		return "warning"
	}
}

func composeFaultDesc(symptom, cause, suggestion string) string {
	var b strings.Builder
	if symptom != "" {
		b.WriteString("【故障表现】\n" + symptom + "\n\n")
	}
	if cause != "" {
		b.WriteString("【可能原因】\n" + cause + "\n\n")
	}
	if suggestion != "" {
		b.WriteString("【处置建议】\n" + suggestion)
	}
	return strings.TrimSpace(b.String())
}

func dedupIDs(ids []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(ids))
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func nodeTypeLabel(t string) string {
	for _, m := range model.NodeTypes {
		if m.Type == t {
			return m.Label
		}
	}
	return t
}

func relLabel(t string) string {
	for _, m := range model.RelTypes {
		if m.Type == t {
			return m.Label
		}
	}
	return t
}
