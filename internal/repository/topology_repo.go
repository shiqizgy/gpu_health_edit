package repository

import (
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// TopologyRepo 拓扑仓储（集群-节点-GPU 三级）
type TopologyRepo struct{ db *gorm.DB }

func NewTopologyRepo(db *gorm.DB) *TopologyRepo { return &TopologyRepo{db: db} }

func (r *TopologyRepo) ListClusters() ([]model.Cluster, error) {
	var out []model.Cluster
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

func (r *TopologyRepo) CreateCluster(c *model.Cluster) error { return r.db.Create(c).Error }

// ListNodesByCluster 列出某集群下的节点（拓扑展开子层用）
func (r *TopologyRepo) ListNodesByCluster(clusterID uint64) ([]model.Node, error) {
	var out []model.Node
	err := r.db.Where("cluster_id = ?", clusterID).Order("id").Find(&out).Error
	return out, err
}

func (r *TopologyRepo) CreateNode(n *model.Node) error { return r.db.Create(n).Error }

// ListGPUsByNode 列出某节点下的 GPU（拓扑叶子层）
func (r *TopologyRepo) ListGPUsByNode(nodeID uint64) ([]model.GPUCard, error) {
	var out []model.GPUCard
	err := r.db.Where("node_id = ?", nodeID).Order("gpu_index").Find(&out).Error
	return out, err
}

// ListGPUsByCluster 列出某集群下的 GPU（健康值明细页用，支持分页）
func (r *TopologyRepo) ListGPUsByCluster(clusterID uint64, limit, offset int) ([]model.GPUCard, int64, error) {
	var out []model.GPUCard
	var total int64
	r.db.Model(&model.GPUCard{}).Where("cluster_id = ?", clusterID).Count(&total)
	err := r.db.Where("cluster_id = ?", clusterID).
		Order("node_id, gpu_index").Limit(limit).Offset(offset).Find(&out).Error
	return out, total, err
}

// UpsertGPU 动态扩容：新增或更新 GPU 卡（按 uuid 唯一）
func (r *TopologyRepo) UpsertGPU(g *model.GPUCard) error {
	return r.db.Where("uuid = ?", g.UUID).
		Assign(map[string]any{
			"node_id":    g.NodeID,
			"cluster_id": g.ClusterID,
			"gpu_index":  g.GPUIndex,
			"model":      g.Model,
			"status":     g.Status,
		}).FirstOrCreate(g).Error
}

// SetGPUStatus 动态缩容：下线/维护一张卡
func (r *TopologyRepo) SetGPUStatus(uuid, status string) error {
	return r.db.Model(&model.GPUCard{}).Where("uuid = ?", uuid).
		Update("status", status).Error
}

// CountGPU 统计 GPU 总数
func (r *TopologyRepo) CountGPU() (int64, error) {
	var n int64
	err := r.db.Model(&model.GPUCard{}).Where("status = ?", "online").Count(&n).Error
	return n, err
}

// AllOnlineGPUs 返回所有在线 GPU（仿真服务初始化拓扑时用）
func (r *TopologyRepo) AllOnlineGPUs() ([]model.GPUCard, error) {
	var out []model.GPUCard
	err := r.db.Where("status = ?", "online").Find(&out).Error
	return out, err
}

// GPUMeta 给故障池用：uuid -> 节点主机名 + 集群名 + 集群ID
type GPUMeta struct {
	NodeHost    string
	ClusterName string
	ClusterID   uint64
}

// GPUMetaMap 返回所有在线 GPU 的 uuid -> {节点主机名, 集群名, 集群ID} 映射。
func (r *TopologyRepo) GPUMetaMap() (map[string]GPUMeta, error) {
	type row struct {
		UUID        string `gorm:"column:uuid"`
		NodeHost    string `gorm:"column:node_host"`
		ClusterName string `gorm:"column:cluster_name"`
		ClusterID   uint64 `gorm:"column:cluster_id"`
	}
	var rows []row
	err := r.db.Table("gpu_card AS g").
		Select("g.uuid, n.hostname AS node_host, c.name AS cluster_name, g.cluster_id").
		Joins("JOIN node n ON n.id = g.node_id").
		Joins("JOIN cluster c ON c.id = g.cluster_id").
		Where("g.status = ?", "online").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]GPUMeta, len(rows))
	for _, x := range rows {
		m[x.UUID] = GPUMeta{NodeHost: x.NodeHost, ClusterName: x.ClusterName, ClusterID: x.ClusterID}
	}
	return m, nil
}

func (r *TopologyRepo) DB() *gorm.DB {
	return r.db
}

// GPUWithStrategy 给评分服务使用:每张卡带上自己的 strategy_id 和所属集群的 strategy_id
type GPUWithStrategy struct {
	UUID              string  `gorm:"column:uuid"`
	ClusterID         uint64  `gorm:"column:cluster_id"`
	CardStrategyID    *uint64 `gorm:"column:card_strategy_id"`
	ClusterStrategyID *uint64 `gorm:"column:cluster_strategy_id"`
}

// AllOnlineGPUsWithStrategy 查所有在线 GPU,带卡级和集群级 strategy_id
func (r *TopologyRepo) AllOnlineGPUsWithStrategy() ([]GPUWithStrategy, error) {
	var out []GPUWithStrategy
	err := r.db.Table("gpu_card AS g").
		Select("g.uuid, g.cluster_id, g.strategy_id AS card_strategy_id, c.strategy_id AS cluster_strategy_id").
		Joins("JOIN cluster c ON c.id = g.cluster_id").
		Where("g.status = ?", "online").
		Scan(&out).Error
	return out, err
}

// BindClusterStrategy 给集群绑定策略
func (r *TopologyRepo) BindClusterStrategy(clusterID uint64, strategyID *uint64) error {
	return r.db.Model(&model.Cluster{}).Where("id = ?", clusterID).
		Update("strategy_id", strategyID).Error
}

// BindGPUStrategy 给单卡绑定策略
func (r *TopologyRepo) BindGPUStrategy(uuid string, strategyID *uint64) error {
	return r.db.Model(&model.GPUCard{}).Where("uuid = ?", uuid).
		Update("strategy_id", strategyID).Error
}

// CountStrategyUsage 统计某策略被多少集群和卡绑定(删除策略前检查用)
func (r *TopologyRepo) CountStrategyUsage(strategyID uint64) (clusterCnt, gpuCnt int64) {
	r.db.Model(&model.Cluster{}).Where("strategy_id = ?", strategyID).Count(&clusterCnt)
	r.db.Model(&model.GPUCard{}).Where("strategy_id = ?", strategyID).Count(&gpuCnt)
	return
}
