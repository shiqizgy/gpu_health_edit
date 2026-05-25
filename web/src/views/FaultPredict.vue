<template>
  <div class="placeholder">
    <div class="ph-card">
      <div class="ph-icon">◷</div>
      <div class="ph-title">故障预测</div>
      <div class="ph-desc">
        本模块计划通过训练模型 / 接入外部 API 实现故障预测。<br />
        按项目规划，此功能<strong>暂未实现</strong>，预留接入位。
      </div>
      <div class="ph-plan">
        <div class="ph-plan-title">规划方向</div>
        <ul>
          <li>基于历史健康快照训练时序模型，预测未来 24-72h 故障概率</li>
          <li>参考业内做法：ECC 单比特错误速率上升趋势可提前预警</li>
          <li>计划以外部 API 形式接入，前端展示预测结果与置信度</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.placeholder { display: flex; justify-content: center; padding-top: 60px; }
.ph-card {
  max-width: 560px; text-align: center;
  background: var(--bg-1); border: 1px solid var(--border);
  border-radius: 12px; padding: 48px;
}
.ph-icon { font-size: 48px; color: var(--accent); opacity: 0.6; margin-bottom: 16px; }
.ph-title { font-size: 22px; font-weight: 700; margin-bottom: 14px; }
.ph-desc { color: var(--text-1); line-height: 1.8; font-size: 14px; }
.ph-plan { margin-top: 28px; text-align: left; background: var(--bg-0); border-radius: 8px; padding: 20px 24px; }
.ph-plan-title { font-size: 12px; letter-spacing: 0.08em; color: var(--text-2); margin-bottom: 12px; text-transform: uppercase; }
.ph-plan ul { margin: 0; padding-left: 18px; color: var(--text-1); line-height: 1.9; font-size: 13px; }
</style>
