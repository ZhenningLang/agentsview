// 中文文案。键名按区域分组,新增 surface 时在此追加。
export const zh: Record<string, string> = {
  // 通用
  "common.save": "保存",
  "common.saving": "保存中…",
  "common.add": "添加",
  "common.remove": "删除",
  "common.enabled": "已启用",
  "common.disabled": "未启用",
  "common.loading": "加载中…",
  "common.localOnly": "此设置仅在本地服务器连接下可用。",

  // 语言切换
  "lang.label": "语言",
  "lang.zh": "中文",
  "lang.en": "English",

  // LLM 配置区
  "llm.title": "LLM 配置",
  "llm.desc": "为各用途配置模型来源。未绑定的用途使用默认配置(当前 [llm])。",
  "llm.providerPool": "Provider 池",
  "llm.providerPoolDesc": "在这里配置模型来源(账号/地址/模型)。每个用途从这里选择。",
  "llm.usageBindings": "用途绑定",
  "llm.usageBindingsDesc": "每个用途使用哪个 provider。留空 = 使用默认配置。",
  "llm.addProvider": "添加 Provider",
  "llm.newProviderName": "provider 名称(如 deepseek-chat)",
  "llm.providerEmptyState": "还没有配置 provider。不配置则所有用途使用默认 [llm];点「添加 Provider」可为某个用途指定独立模型。",
  "llm.defaultOption": "默认([llm] 配置)",
  "llm.saveConfig": "保存 LLM 配置",
  "llm.saved": "LLM 配置已保存",
  "llm.saveFailed": "保存 LLM 配置失败",
  "llm.loadFailed": "加载 LLM 配置失败",
  "llm.dangling": "下列用途绑定了不存在的 provider,已回退到默认:",

  // provider 字段
  "provider.baseUrl": "Base URL",
  "provider.apiKey": "API Key",
  "provider.model": "模型",
  "provider.reasoningEffort": "推理强度",
  "provider.balanceUrl": "余额查询 URL",
  "provider.enabled": "启用",

  // 用途(business)
  "usage.enrich": "会话增强",
  "usage.enrich.desc": "为会话生成标题/摘要/关键词。",
  "usage.extract": "记忆抽取",
  "usage.extract.desc": "从会话中用 LLM 提炼候选记忆。",
  "usage.consolidate": "记忆巩固",
  "usage.consolidate.desc": "把暂存候选巩固进长期记忆的决策模型。",
  "usage.embed": "向量 Embedding",
  "usage.embed.desc": "为语义检索/召回生成向量(通常用专门的 embedding provider)。",
  "usage.recall_rerank": "召回重排",
  "usage.recall_rerank.desc": "对召回的记忆做相关性重排(可选,留空则不重排)。",

  // 记忆巩固卡(瘦身后)
  "consolidate.title": "记忆巩固",
  "consolidate.desc": "后台定时把暂存候选巩固进长期记忆。默认关闭,开启后按间隔运行。",
  "consolidate.interval": "运行间隔",
  "consolidate.modelHint": "用哪个模型?在上方「LLM 配置」里设置 consolidate 的绑定。",
  "consolidate.stateSaved": "巩固开关已保存",
  "consolidate.intervalSaved": "巩固间隔已保存",
  "consolidate.toggleFailed": "切换巩固开关失败",
  "consolidate.saveFailed": "保存巩固设置失败",

  // 记忆质量面板
  "quality.title": "Memory 机制运行",
  "quality.caveat": "非零指标只证明埋点接通,不代表召回质量达标。",
};
