package repository

import (
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// TopologyRepo 拓扑仓储（集群-节点-GPU 三级）
type TopologyRepo struct{ db *gorm.DB }

func NewTopologyRepo(db *gorm.DB) *TopologyRepo { return &TopologyRepo{db: db} }

// ---- 集群 ----
func (r *TopologyRepo) ListClusters() ([]model.Cluster, error) {
	var out []model.Cluster
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

func (r *TopologyRepo) CreateCluster(c *model.Cluster) error { return r.db.Create(c).Error }

// ---- 节点 ----
// ListNodesByCluster 列出某集群下的节点（拓扑展开子层用）
func (r *TopologyRepo) ListNodesByCluster(clusterID uint64) ([]model.Node, error) {
	var out []model.Node
	err := r.db.Where("cluster_id = ?", clusterID).Order("id").Find(&out).Error
	return out, err
}

func (r *TopologyRepo) CreateNode(n *model.Node) error { return r.db.Create(n).Error }

// ---- GPU 卡 ----
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
