<template>
  <div class="placeholder">
    <div class="ph-card">
      <div class="ph-icon">⌖</div>
      <div class="ph-title">故障根因分析</div>
      <div class="ph-desc">
        本模块计划做基于规则的根因分析，并以 AI 助手形式接入外部 API。<br />
        按项目规划，此功能<strong>暂未实现</strong>，预留接入位。
      </div>
      <div class="ph-plan">
        <div class="ph-plan-title">规划方向（先做基于规则的根因分析）</div>
        <ul>
          <li>规则示例：分数骤降 + 温度过高 + 降频事件 → 散热异常</li>
          <li>规则示例：ECC DBE ≥ 1 + 任务崩溃 → 显存物理损伤，建议 RMA</li>
          <li>规则示例：XID 79 + PCIe 重传升高 → GPU 掉总线，建议冷重启</li>
          <li>后续接外部 API，做成对话式 AI 助手，结合健康值指标数据定位根因</li>
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
