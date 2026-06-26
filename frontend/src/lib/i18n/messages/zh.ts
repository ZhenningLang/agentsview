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
  "common.provider": "Provider",
  "common.test": "测试",
  "common.testing": "测试中…",

  // LLM enrichment(合并后的单一 LLM 配置区)
  "enrich.title": "LLM 配置",
  "enrich.desc": "统一管理「模型来源(Providers)」与「用途分配」:先配置 provider,再为每个用途选一个。",
  "feature.enrichTitle": "会话增强",
  "feature.enrichDesc": "为本地会话生成标题/摘要/关键词。所用模型见上方「LLM 配置」里「会话增强」用途的分配。",
  "feature.enrichSave": "保存增强设置",
  "enrich.enable": "启用 LLM enrichment",
  "enrich.scheduling": "调度",
  "enrich.minMsgs": "最少用户消息数",
  "enrich.reenrichDelta": "重新增强消息增量",
  "enrich.idleMinutes": "空闲分钟数",
  "enrich.concurrency": "并发数",
  "enrich.runPeriodically": "定时运行",
  "enrich.save": "保存 LLM 配置",
  "enrich.test": "测试连接",
  "enrich.testing": "测试中…",
  "enrich.saved": "LLM 配置已保存",
  "enrich.run": "运行 enrichment",
  "enrich.stop": "停止",
  "enrich.refresh": "刷新状态",

  // Providers(模型来源 · 厂商+Key,可多实例)
  "providers.title": "Providers · 模型来源",
  "providers.desc": "每个 = 名字(可改) + 厂商 + API Key。同一厂商可建多个(不同 Key/账号)。模型在下方「用途分配」里填。",
  "providers.name": "名称",
  "providers.vendor": "厂商",
  "providers.customVendor": "自定义",
  "providers.add": "添加 provider",
  "providers.empty": "还没有 provider。点「添加 provider」配一个(厂商 + Key)。",

  // 用途分配(每个用途:选 provider + 填模型)
  "assign.title": "用途分配",
  "assign.desc": "每个用途:选一个已配好的 provider + 填该用途用的模型。",
  "assign.use": "用",
  "assign.noProvider": "(先在上面配一个 provider)",
  "assign.dangling": "下列用途绑定了不存在的 provider,已回退到默认:",

  // 语言切换
  "lang.label": "语言",
  "lang.zh": "中文",
  "lang.en": "English",

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
