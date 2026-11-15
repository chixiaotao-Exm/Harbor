import { computed, ref } from 'vue'

export type Lang = 'zh' | 'en'

const STORAGE_KEY = 'harbor_lang'

function initialLang(): Lang {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'zh' || saved === 'en') return saved
  // 跟随浏览器，中文环境默认中文
  return (navigator.language || '').toLowerCase().startsWith('zh') ? 'zh' : 'en'
}

export const lang = ref<Lang>(initialLang())

export function setLang(next: Lang) {
  lang.value = next
  localStorage.setItem(STORAGE_KEY, next)
}

export function toggleLang() {
  setLang(lang.value === 'zh' ? 'en' : 'zh')
}

type Dict = Record<string, [zh: string, en: string]>

// 每个键的顺序是 [中文, English]
const dict: Dict = {
  'app.subtitle': ['批量开户工作台', 'Bulk account console'],

  'nav.group.run': ['运行', 'Run'],
  'nav.workbench': ['工作台', 'Workbench'],
  'nav.async': ['异步任务', 'Async jobs'],
  'nav.group.data': ['数据', 'Data'],
  'nav.accounts': ['账号', 'Accounts'],
  'nav.group.config': ['配置', 'Config'],
  'nav.settings': ['设置', 'Settings'],
  'nav.foot': ['已绑双因子 {mfa} · 未绑 {plain}', '{mfa} with 2FA · {plain} without'],

  'work.title': ['工作台', 'Workbench'],
  'work.sub': [
    '一条任务一个出口，取号串行，跑完一单再补下一单',
    'One exit per task, mailboxes taken one at a time',
  ],
  'work.start': ['开工', 'Start'],
  'work.stop': ['停止', 'Stop'],
  'work.balance': ['余额', 'Balance'],
  'work.ok': ['成功', 'Succeeded'],
  'work.fail': ['失败', 'Failed'],
  'work.rate': ['成功率', 'Success rate'],
  'work.params': ['参数', 'Parameters'],
  'work.count': ['数量', 'Count'],
  'work.threads': ['并发', 'Concurrency'],
  'work.delay': ['间隔（秒）', 'Delay (s)'],
  'work.domain': ['邮箱域名', 'Mail domain'],
  'work.region': ['出口地区', 'Exit region'],
  'work.regionBusy': ['切换中', 'switching'],
  'work.autoEnqueue': [
    '开户成功后自动投递到异步任务（绑双因子）',
    'Send new accounts to the async 2FA queue automatically',
  ],
  'work.hint': [
    '数量是总配额，并发是同时在跑的槽位；任一时刻在手邮箱不会超过并发数。',
    'Count is the total quota, concurrency the number of slots in flight; mailboxes held at once never exceed concurrency.',
  ],
  'work.logs': ['日志', 'Log'],
  'work.newestFirst': ['最新在上', 'newest first'],
  'work.logEmpty': [
    '开工后这里会逐条显示谁在做什么',
    'Each account and step will show up here once you start',
  ],
  'work.items': ['工作明细', 'Job detail'],
  'work.col.email': ['账号', 'Account'],
  'work.col.ip': ['IP', 'IP'],
  'work.col.stage': ['环节', 'Stage'],
  'work.col.state': ['状态', 'Status'],
  'work.itemsEmpty': ['还没有明细', 'Nothing here yet'],
  'work.state.ok': ['成功', 'OK'],
  'work.state.fail': ['失败', 'Failed'],
  'work.state.running': ['进行中', 'Running'],
  'work.regionChanged': ['出口地区已切换为 {region}', 'Exit region switched to {region}'],
  'work.startFailed': ['启动失败', 'Failed to start'],

  'settings.title': ['设置', 'Settings'],
  'settings.sub': [
    '密钥与代理只存在本机，接口返回打码',
    'Keys and proxy stay on this machine; the API always masks them',
  ],
  'settings.sms': ['接码平台', 'SMS platform'],
  'settings.apiKey': ['API Key', 'API Key'],
  'settings.apiKeyPh': ['粘贴 API Key', 'Paste your API key'],
  'settings.keepExisting': ['留空则沿用已保存的值', 'Leave empty to keep the saved value'],
  'settings.proxy': [
    '代理（socks5://用户-region-XX-sid-XXXX-密码@主机:端口）',
    'Proxy (socks5://user-region-XX-sid-XXXX:pass@host:port)',
  ],
  'settings.proxyHint': [
    '多余空格、换行、全角冒号会自动清掉；不写协议自动补 socks5h://',
    'Stray spaces, line breaks and full-width colons are cleaned; socks5h:// is added when missing',
  ],
  'settings.region': ['出口地区', 'Exit region'],
  'settings.regionHint': [
    '写入代理串的 region-XX 段，工作台可直接切换',
    'Written into the proxy as region-XX; the workbench can switch it too',
  ],
  'settings.defaults': ['默认行为', 'Defaults'],
  'settings.defaultCount': ['默认数量', 'Default count'],
  'settings.defaultDelay': ['默认间隔（秒）', 'Default delay (s)'],
  'settings.addPassword': ['开户时写入随机密码', 'Set a random password on signup'],
  'settings.addMFA': ['开户后自动投双因子队列', 'Queue new accounts for 2FA'],
  'settings.save': ['保存', 'Save'],
  'settings.saved': ['已保存', 'Saved'],
  'settings.saveFailed': ['保存失败', 'Save failed'],

  'acc.title': ['账号', 'Accounts'],
  'acc.sub': [
    '已绑与未绑分开两个页签，各自分页与勾选',
    'Bound and unbound accounts live in separate tabs with their own paging and selection',
  ],
  'acc.export': ['导出 txt', 'Export txt'],
  'acc.push': ['投递到异步', 'Send to async'],
  'acc.tab.mfa': ['已绑双因子', 'With 2FA'],
  'acc.tab.plain': ['未绑双因子', 'Without 2FA'],
  'acc.search': ['按邮箱搜索', 'Search by email'],
  'acc.perPage': ['每页 {n}', '{n} per page'],
  'acc.selectAll': ['选全部 {n} 条', 'Select all {n}'],
  'acc.clearPick': ['清空勾选', 'Clear selection'],
  'acc.selected': ['已勾 {n}', '{n} selected'],
  'acc.col.email': ['账号', 'Account'],
  'acc.col.password': ['密码', 'Password'],
  'acc.col.secret': ['双因子密钥', '2FA secret'],
  'acc.col.code': ['动态码', 'Code'],
  'acc.col.state': ['状态', 'Status'],
  'acc.col.actions': ['操作', 'Actions'],
  'acc.getCode': ['取 6 位码', 'Get 6-digit code'],
  'acc.copyRow': ['整行', 'Row'],
  'acc.state.bound': ['已绑', 'Bound'],
  'acc.state.fail': ['失败', 'Failed'],
  'acc.pageInfo': ['共 {total} 条 · 第 {page}/{pages} 页', '{total} total · page {page}/{pages}'],
  'acc.prev': ['上一页', 'Prev'],
  'acc.next': ['下一页', 'Next'],
  'acc.emptyPage': ['这一页没有账号', 'No accounts on this page'],
  'acc.copied': ['{label}已复制', '{label} copied'],
  'acc.copiedEmail': ['账号', 'Account'],
  'acc.copiedPassword': ['密码', 'Password'],
  'acc.copiedSecret': ['密钥', 'Secret'],
  'acc.copiedCode': ['动态码', 'Code'],
  'acc.copiedRow': ['整行', 'Row'],
  'acc.pushed': ['已投递 {n} 条到异步任务', 'Sent {n} account(s) to async jobs'],
  'acc.pushFailed': ['投递失败', 'Send failed'],
  'acc.codeFailed': ['取码失败', 'Failed to get code'],
  'acc.goAsync': ['去看异步任务 →', 'Open async jobs →'],
  'acc.close': ['关闭', 'Close'],
  'acc.exported': ['已导出 {n} 条', 'Exported {n}'],

  'async.title': ['异步任务', 'Async jobs'],
  'async.sub': [
    '每步失败会自动重试 3 次并换出口，超限落失败页',
    'Each step retries 3 times with a new exit; failures land in the failed list',
  ],
  'async.pushAll': ['投递未绑账号', 'Send unbound accounts'],
  'async.retryAll': ['全部重试', 'Retry all'],
  'async.logs': ['日志', 'Log'],
  'async.logEmpty': [
    '队列跑起来后这里会显示每一步在做什么',
    'Once the queue runs, each step shows up here',
  ],
  'async.items': ['工作明细', 'Job detail'],
  'async.failed': ['失败页（{n}）', 'Failed ({n})'],
  'async.selectAll': ['全选', 'Select all'],
  'async.unselectAll': ['取消全选', 'Clear'],
  'async.retryPick': ['重试选中', 'Retry selected'],
  'async.col.email': ['账号', 'Account'],
  'async.col.reason': ['原因', 'Reason'],
  'async.col.times': ['次数', 'Tries'],
  'async.noFail': ['没有失败', 'No failures'],
  'async.queued': ['已投递 {n} 条', 'Queued {n}'],
  'async.retried': ['已重投 {n} 条', 'Requeued {n}'],
  'async.q1': ['Q1 校验登录态', 'Q1 Check session'],
  'async.q2': ['Q2 账号密码登录', 'Q2 Password login'],
  'async.q3': ['Q3 提交双因子登记', 'Q3 Enroll 2FA'],
  'async.q4': ['Q4 动态码激活', 'Q4 Activate code'],
  'async.q5': ['Q5 收尾确认', 'Q5 Finalize'],
  'async.empty': ['空', 'empty'],

  'lang.switch': ['EN', '中文'],
  'lang.title': ['切换语言', 'Switch language'],

  'acc.col.region': ['地区', 'Region'],
  'acc.col.session': ['会话', 'Session'],
  'acc.refreshSession': ['刷新会话', 'Refresh session'],
  'acc.sessionDone': ['已刷新 {n} 个，失败 {f} 个', 'Refreshed {n}, failed {f}'],
  'acc.sessionNone': ['未刷新', 'not refreshed'],
  'acc.sessionOk': ['已刷新', 'refreshed'],
  'acc.col.price': ['优惠价', 'Promo price'],
  'acc.checkPrice': ['查优惠价', 'Check price'],
  'acc.priced': ['已查 {n} 个账号', 'Priced {n} accounts'],
  'acc.priceFailed': ['查价失败', 'Price lookup failed'],
  'acc.priceNone': ['未查', 'not checked'],
  'acc.pricePromo': ['有优惠', 'promo'],
  'region.none': ['没有匹配的地区', 'No matching region'],
  'region.count': ['{n} 个地区', '{n} regions'],
}

/** Translate one key. Extra args fill {placeholders} in order. */
export function t(key: string, ...args: (string | number)[]): string {
  const entry = dict[key]
  let text = entry ? entry[lang.value === 'zh' ? 0 : 1] : key
  if (args.length) {
    let i = 0
    text = text.replace(/\{(\w+)\}/g, () => String(args[i++]))
    // 具名参数也支持：调用方传对象时按序替换
  }
  return text
}

/** Fills {name} placeholders from an object. */
export function tf(key: string, vars: Record<string, string | number>): string {
  const entry = dict[key]
  let text = entry ? entry[lang.value === 'zh' ? 0 : 1] : key
  return text.replace(/\{(\w+)\}/g, (_, name) => String(vars[name] ?? name))
}

/** Region label for the current language, e.g. "美国 · US". */
export function regionLabel(r: { code: string; zh: string; en: string }): string {
  const name = lang.value === 'zh' ? r.zh : r.en
  return `${name} · ${r.code}`
}

/** Reactive helper so templates re-render on language change. */
export function useI18n() {
  const current = computed(() => lang.value)
  return { lang, current, t, tf, setLang, toggleLang, regionLabel }
}
