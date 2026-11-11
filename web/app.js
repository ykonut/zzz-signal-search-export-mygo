'use strict'

/* ------------------------------------------------------------------ *
 * 轻量版前端：原生 ES Module + 手写 SVG 图表，无框架、无构建步骤。
 * 核心统计逻辑移植自上游 gachaDetail.js
 * ------------------------------------------------------------------ */

const $ = (id) => document.getElementById(id)

const state = {
  version: '',
  config: {},
  i18n: null,
  langMap: [],
  dataMap: new Map(),
  current: 0,
  resources: {},
  icons: {},
  status: 'init',
  log: '',
  busy: false
}

/* ----------------------------- 工具 ----------------------------- */
const api = async (path, options = {}) => {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...options
  })
  return res.json()
}

const escapeHtml = (str) => String(str ?? '').replace(/[&<>"']/g, (c) => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[c]))


/* ----------------------- 历史记录智能配色 ----------------------- */
/* 思路：从角色头像提取主色相，再把色相吸附到一组经过挑选的
   高可读性调色板；同一张卡片内贪心地避免色相重复，保证彼此可区分。 */
const HISTORY_COLOR_KEY = 'zzz.historyColors.v8'
const colorCache = new Map()
try {
  const raw = localStorage.getItem(HISTORY_COLOR_KEY)
  if (raw) {
    const obj = JSON.parse(raw)
    for (const k of Object.keys(obj || {})) {
      const v = Number(obj[k])
      if (isFinite(v)) colorCache.set(String(k), v)
    }
  }
} catch (e) {}

const saveColorCache = () => {
  try {
    const obj = {}
    colorCache.forEach((v, k) => { obj[k] = v })
    localStorage.setItem(HISTORY_COLOR_KEY, JSON.stringify(obj))
  } catch (e) {}
}

const PALETTE_HEX = ['#dc2626', '#ea580c', '#d97706', '#65a30d', '#16a34a', '#0d9488', '#0891b2', '#2563eb', '#4f46e5', '#7c3aed', '#c026d3', '#db2777']
const hashHue = (str) => {
  let h = 0
  const s = String(str || '')
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) % 360
  return Math.abs(h) % 360
}

const hueDistance = (a, b) => {
  const d = Math.abs(a - b) % 1
  return Math.min(d, 1 - d)
}

const pickPaletteIndex = (hue, used) => {
  const order = PALETTE_HUES.map((_, i) => i)
    .sort((a, b) => hueDistance(PALETTE_HUES[a], hue) - hueDistance(PALETTE_HUES[b], hue))
  for (const idx of order) if (!used.has(idx)) return idx
  return order[0]
}

// 区分模式专用：在接近头像色相的前提下，保证与已用颜色有足够色相间隔
const pickDistinctIndex = (hue, used) => {
  const candidates = PALETTE_HUES.map((ph, i) => ({ i, d: hueDistance(ph, hue) }))
    .sort((a, b) => a.d - b.d)
  let fallback = candidates[0].i
  for (const c of candidates) {
    if (used.has(c.i)) continue
    fallback = c.i
    let minSep = 1
    used.forEach((u) => { minSep = Math.min(minSep, hueDistance(PALETTE_HUES[c.i], PALETTE_HUES[u])) })
    if (used.size === 0 || minSep >= 0.13) return c.i
  }
  return fallback
}

const rgbToHsl = (r, g, b) => {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b)
  let h = 0, s = 0
  const l = (max + min) / 2
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    if (max === r) h = (g - b) / d + (g < b ? 6 : 0)
    else if (max === g) h = (b - r) / d + 2
    else h = (r - g) / d + 4
    h /= 6
  }
  return [h, s, l]
}

const hexToHsl = (hex) => {
  const m = String(hex).replace('#', '')
  const r = parseInt(m.slice(0, 2), 16)
  const g = parseInt(m.slice(2, 4), 16)
  const b = parseInt(m.slice(4, 6), 16)
  return rgbToHsl(r, g, b)
}

const PALETTE_HUES = PALETTE_HEX.map((h) => hexToHsl(h)[0])

const hslToHex = (h, s, l) => {
  const hue2rgb = (p, q, t) => {
    if (t < 0) t += 1
    if (t > 1) t -= 1
    if (t < 1 / 6) return p + (q - p) * 6 * t
    if (t < 1 / 2) return q
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6
    return p
  }
  let r, g, b
  if (s === 0) { r = g = b = l } else {
    const q = l < 0.5 ? l * (1 + s) : l + s - l * s
    const p = 2 * l - q
    r = hue2rgb(p, q, h + 1 / 3)
    g = hue2rgb(p, q, h)
    b = hue2rgb(p, q, h - 1 / 3)
  }
  const toHex = (x) => Math.round(x * 255).toString(16).padStart(2, '0')
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`
}

const fallbackHue = (key) => hashHue(key) / 360

const initialColor = (name, key) => {
  const idx = pickPaletteIndex(fallbackHue(key || name), new Set())
  return hslToHex(PALETTE_HUES[idx], 0.34, 0.46)
}

const extractDominantHue = (img, key) => {
  try {
    if (!img || !img.naturalWidth) return null
    const size = 48
    const canvas = document.createElement('canvas')
    canvas.width = size
    canvas.height = size
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    ctx.drawImage(img, 0, 0, size, size)
    const data = ctx.getImageData(0, 0, size, size).data

    const cands = []
    for (let i = 0; i < data.length; i += 4) {
      if (data[i + 3] < 128) continue
      const r = data[i], g = data[i + 1], b = data[i + 2]
      const [h, sat, light] = rgbToHsl(r, g, b)
      if (light < 0.12 || light > 0.9) continue
      if (sat < 0.4) continue
      cands.push({ sat, h, r, g, b })
    }
    if (!cands.length) return null

    // 取饱和度最高的一批像素（发色 / 眼睛 / 配饰），避免平均值被肤色带偏
    cands.sort((a, b) => b.sat - a.sat)
    const take = Math.max(1, Math.min(80, Math.floor(cands.length * 0.15)))
    // 环形平均：把色相转成向量，避免 0/1 边界问题
    let x = 0, y = 0, avgSat = 0, wSum = 0
    for (let i = 0; i < take; i++) {
      const c = cands[i]
      const w = c.sat * c.sat
      x += Math.cos(c.h * Math.PI * 2) * w
      y += Math.sin(c.h * Math.PI * 2) * w
      avgSat += c.sat * w
      wSum += w
    }
    avgSat = avgSat / (wSum || 1)
    let h = Math.atan2(y, x) / (Math.PI * 2)
    if (h < 0) h += 1
    if (key) {
      const shift = ((hashHue(String(key)) % 13) - 6) / 360
      h = (h + shift + 1) % 1
    }
    return h
  } catch (e) {
    return null
  }
}

const applyHistoryColor = (row, color) => {
  const nameEl = row.querySelector('.history-name')
  const numEl = row.querySelector('.pity-num')
  const fillEl = row.querySelector('.pity-bar-fill')
  if (nameEl) nameEl.style.color = color
  if (numEl) numEl.style.color = color
  if (fillEl) fillEl.style.background = color
}

const assignCardColors = (card) => {
  const rows = Array.from(card.querySelectorAll('.history-row[data-key]'))
  if (!rows.length) return
  const groups = new Map()
  rows.forEach((row) => {
    const key = row.getAttribute('data-key')
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key).push(row)
  })

  const resolved = new Map()
  groups.forEach((groupRows, key) => {
    if (colorCache.has(key)) { resolved.set(key, colorCache.get(key)); return }
    resolved.set(key, fallbackHue(key))
  })

  const applyAll = () => {
    const usedIdx = new Set()
    groups.forEach((groupRows, key) => {
      // 区分调色板：同卡片内优先使用未占用的色相，保证每条记录可区分
      const idx = pickDistinctIndex(resolved.get(key), usedIdx)
      usedIdx.add(idx)
      const color = hslToHex(PALETTE_HUES[idx], 0.34, 0.46)
      groupRows.forEach((row) => applyHistoryColor(row, color))
    })
  }
  // 图片加载后提取真实主色相，并重新分配整张卡片的配色
  groups.forEach((groupRows, key) => {
    if (colorCache.has(key)) return
    const img = groupRows[0].querySelector('img.history-avatar')
    if (!img) return
    const run = () => {
      const hue = extractDominantHue(img, key)
      if (hue == null) return
      colorCache.set(key, hue)
      saveColorCache()
      resolved.set(key, hue)
      applyAll()
    }
    if (img.complete && img.naturalWidth) run()
    else img.addEventListener('load', run, { once: true })
  })
}

const assignAllCardColors = (container) => {
  container.querySelectorAll('.card').forEach((card) => assignCardColors(card))
}

const t = (path, fallback = '') => {
  const parts = path.split('.')
  let cur = state.i18n
  for (const p of parts) {
    if (cur == null) return fallback
    cur = cur[p]
  }
  return cur == null ? fallback : cur
}

const normalizeCurrent = (v) => {
  if (v === 0 || v === '0' || v == null || v === '') return 0
  return String(v)
}

const hydrateData = (payload) => {
  const map = new Map()
  for (const d of payload.data || []) {
    map.set(d.uid, {
      ...d,
      result: new Map(d.result || []),
      typeMap: new Map(d.typeMap || [])
    })
  }
  state.current = normalizeCurrent(payload.current)
  return map
}

/* --------------------------- 统计逻辑 --------------------------- */
const weaponTypeNames = new Set(['音擎', 'W-Engines'])
const bangbooTypeNames = new Set(['邦布', 'Bangboo'])
const characterTypeNames = new Set(['代理人', 'Agents'])
const isCharacter = (n) => characterTypeNames.has(n)
const isWeapon = (n) => weaponTypeNames.has(n)
const isBangboo = (n) => bangbooTypeNames.has(n)

const order = ['2', '3', '1', '5', '102', '103']

const itemCount = (map, name) => map.set(name, (map.get(name) || 0) + 1)

const gachaDetail = (data) => {
  const detailMap = new Map()
  for (const key of order) {
    if (!data.has(key)) continue
    const value = data.get(key)
    const detail = {
      count2: 0, count3: 0, count4: 0,
      count2w: 0, count3w: 0, count4w: 0, count3c: 0, count4c: 0, count3b: 0, count4b: 0,
      weapon2: new Map(), weapon3: new Map(), weapon4: new Map(),
      char3: new Map(), char4: new Map(),
      bang3: new Map(), bang4: new Map(),
      date: [],
      ssrPos: [], countMio: 0, total: value.length
    }
    let lastSSR = 0
    let dateMin = 0
    let dateMax = 0
    value.forEach((item, index) => {
      const { time, name, item_type: type, rank_type: rank, item_id } = item
      const timestamp = new Date(time).getTime()
      if (!dateMin) dateMin = timestamp
      if (!dateMax) dateMax = timestamp
      if (dateMin > timestamp) dateMin = timestamp
      if (dateMax < timestamp) dateMax = timestamp
      if (rank === '2') {
        detail.count2++
        detail.countMio++
        if (isWeapon(type)) { detail.count2w++; itemCount(detail.weapon2, name) }
      } else if (rank === '3') {
        detail.count3++
        detail.countMio++
        if (isWeapon(type)) { detail.count3w++; itemCount(detail.weapon3, name) }
        else if (isBangboo(type)) { detail.count3b++; itemCount(detail.bang3, name) }
        else if (isCharacter(type)) { detail.count3c++; itemCount(detail.char3, name) }
      } else if (rank === '4') {
        detail.ssrPos.push([name, index + 1 - lastSSR, time, key, item_id, type])
        lastSSR = index + 1
        detail.count4++
        detail.countMio = 0
        if (isWeapon(type)) { detail.count4w++; itemCount(detail.weapon4, name) }
        else if (isBangboo(type)) { detail.count4b++; itemCount(detail.bang4, name) }
        else if (isCharacter(type)) { detail.count4c++; itemCount(detail.char4, name) }
      }
    })
    detail.date = [dateMin, dateMax]
    if (detail.total) detailMap.set(key, detail)
  }
  return detailMap
}

const percent = (num, total) => total ? `${Math.round(num / total * 10000) / 100}%` : '0%'

const avg5 = (list) => {
  if (!list.length) return 0
  let n = 0
  list.forEach((item) => { n += item[1] })
  return parseInt((n / list.length) * 100) / 100
}

/* ----------------------------- 渲染 ----------------------------- */
const currentData = () => state.dataMap.get(state.current)
const visibleEntries = () => [...state.dataMap.values()].filter((d) => !d.deleted)

const maskUid = (uid) => `${uid}`.replace(/(.{3})(.+)(.{3})$/, '$1***$3')

const allowClick = () => {
  const data = currentData()
  if (!data) return true
  return Date.now() - data.time >= 10000
}

const renderHint = () => {
  const el = $('hint')
  el.className = 'hint'
  const failed = t('ui.hint.failed', 'failed')
  if (!state.i18n) { el.textContent = 'Loading...'; return }
  if (state.status === 'loading') {
    el.classList.add('loading')
    el.textContent = state.log || t('ui.hint.init', '')
  } else if (state.status === 'loaded') {
    const data = currentData()
    el.textContent = data
      ? `${t('ui.hint.lastUpdate', '')}${t('symbol.colon', '')}${new Date(data.time).toLocaleString()}`
      : t('ui.hint.init', '')
  } else if (state.status === 'failed') {
    el.classList.add('error')
    el.textContent = `${state.log || ''}${state.log ? ' - ' : ''}${failed}`
  } else {
    el.textContent = t('ui.hint.init', '')
  }
}

const renderUidSelect = () => {
  const sel = $('uid-select')
  const entries = visibleEntries()
  const shouldShow = entries.length > 1 || state.current === 0
  if (!entries.length || !shouldShow) { sel.style.display = 'none'; return }
  sel.style.display = ''
  const opts = []
  if (state.current === 0 || entries.length > 1) {
    // 允许“新账号”入口
    if (state.current === 0) opts.push(`<option value="0" selected>${escapeHtml(t('ui.select.newAccount', 'New account'))}</option>`)
  }
  for (const d of entries) {
    opts.push(`<option value="${d.uid}" ${d.uid === state.current ? 'selected' : ''}>${escapeHtml(maskUid(d.uid))}</option>`)
  }
  sel.innerHTML = opts.join('')
}

const iconCacheKey = () => `${state.version || ''}-${(state.icons && state.icons.cacheTag) || '0'}`

const historyRowHtml = (item, denom) => {
  const name = item[0]
  const pity = item[1]
  const time = item[2]
  const itemId = item[4]
  const type = item[5]
  const showIcon = !!itemId
  const groupKey = showIcon ? String(itemId) : `n:${type}:${name}`
  const color = initialColor(name, groupKey)
  const icon = showIcon
    ? `<img class="history-avatar" src="/api/icon/${encodeURIComponent(itemId)}?v=${encodeURIComponent(iconCacheKey())}" alt="" loading="lazy" onerror="this.style.visibility='hidden'">`
    : `<span class="history-avatar placeholder">${escapeHtml(String(name || '?').slice(0, 1))}</span>`
  const width = Math.max(4, Math.min(100, Math.round((pity / denom) * 100)))
  return `<div class="history-row" data-key="${escapeHtml(groupKey)}" title="${escapeHtml(name)} · ${escapeHtml(new Date(time).toLocaleString())}">
      ${icon}
      <span class="history-name" style="color:${color}">${escapeHtml(name)}</span>
      <span class="pity-bar"><span class="pity-bar-fill" style="width:${width}%;background:${color}"></span></span>
      <span class="pity-num" style="color:${color}">${pity}</span>
    </div>`
}


const rarityLegendHtml = (detail) => {
  const d = state.i18n.ui.data
  const items = [
    { label: d.star4, count: detail.count4, color: '#eab308' },
    { label: d.star3, count: detail.count3, color: '#8b5cf6' },
    { label: d.star2, count: detail.count2, color: '#3b82f6' }
  ]
  return `<div class="rarity-legend">${items.map((x) => `<span class="rarity-legend-item" title="${escapeHtml(x.label)} ${x.count}（${percent(x.count, detail.total)}）"><i style="background:${x.color}"></i>${escapeHtml(x.label)} <b>${x.count}</b></span>`).join('')}</div>`
}

const renderDetail = () => {
  const container = $('detail')
  const data = currentData()
  if (!data) {
    container.innerHTML = `<div class="empty"><div class="big">${escapeHtml(t('ui.hint.init', ''))}</div></div>`
    return
  }
  const detail = gachaDetail(data.result)
  const typeMap = data.typeMap
  const d = state.i18n.ui.data
  const colon = t('symbol.colon', '')
  const cards = []

  for (const [key, detailData] of detail) {
    const typeName = typeMap.get(key) || key
    const pityList = [...detailData.ssrPos].reverse()
    const denom = Math.max(90, ...detailData.ssrPos.map((it) => it[1]), 1)
    const avg = detailData.ssrPos.length ? avg5(detailData.ssrPos) : null
    const currentPity = detailData.countMio
    const currentPityLabel = d.currentPity || '已垫抽数'
    const cpWidth = Math.max(3, Math.min(100, Math.round((currentPity / denom) * 100)))
    const currentPityRow = `<div class="history-row current-pity-row" title="已垫抽数 ${currentPity} 抽">
        <span class="history-avatar placeholder cp-dot">垫</span>
        <span class="history-name cp-label">${escapeHtml(currentPityLabel)}</span>
        <span class="pity-bar"><span class="pity-bar-fill cp-fill" style="width:${cpWidth}%"></span></span>
        <span class="pity-num cp-num">${currentPity}</span>
      </div>`
    const historyHtml = `<div class="history-head">${escapeHtml(d.history)}${colon}</div>
        ${currentPityRow}
        ${detailData.ssrPos.length
          ? `<div class="history-list">${pityList.map((item) => historyRowHtml(item, denom)).join('')}</div>`
          : ''}`


    cards.push(`
      <section class="card">
        <p class="card-title">${escapeHtml(typeName)}</p>
        <div class="detail-meta">
          <span title="${new Date(detailData.date[0]).toLocaleString()}">${new Date(detailData.date[0]).toLocaleDateString()}</span>
          -
          <span title="${new Date(detailData.date[1]).toLocaleString()}">${new Date(detailData.date[1]).toLocaleDateString()}</span>
        </div>
        <div class="top-summary">
          ${avg === null ? `<span class="ts-item">${escapeHtml(d.noRecords || '暂无 S 级记录')}</span>` : `<span class="ts-item">${escapeHtml(d.average)}<b> ${avg}</b></span>`}
        </div>
        <div class="count-row">
          <span class="total-pulls">${escapeHtml(d.total)} <b>${detailData.total}</b> ${escapeHtml(d.times)}</span>
          ${rarityLegendHtml(detailData)}
        </div>
        ${historyHtml}
      </section>`)
  }

  container.innerHTML = cards.length
    ? cards.join('')
    : `<div class="empty"><div class="big">${escapeHtml(t('ui.hint.init', ''))}</div></div>`
  assignAllCardColors(container)
}

const renderTop = () => {
  $('btn-game').textContent = state.busy ? '…' : t('ui.button.readGame', '从游戏读取')
  $('btn-game').disabled = state.busy || !allowClick()
  $('btn-url').textContent = state.busy ? '…' : t('ui.button.fromUrl', '从URL读取')
  $('btn-url').disabled = state.busy || !allowClick()
  $('btn-export').textContent = `${t('ui.button.files', 'Files')} ▾`
  $('cmd-excel').textContent = t('ui.button.excel', 'Excel')
  $('cmd-uigf').textContent = t('ui.button.uigf', 'UIGF')
  $('cmd-import').textContent = t('ui.button.import', 'Import UIGF')
  $('btn-new').textContent = t('ui.button.switchAccount', '切换账号')
  $('btn-new').title = t('ui.button.switchAccount', '切换账号')
  $('btn-setting').title = t('ui.button.setting', 'Settings')
  document.title = t('ui.win.title', '')
}

const renderAll = () => {
  renderTop()
  renderHint()
  renderUidSelect()
  renderDetail()
}

/* --------------------------- 静态文本 --------------------------- */
const applyStaticText = () => {
  $('url-title').textContent = t('ui.urlDialog.title', '')
  $('url-hint').textContent = t('ui.urlDialog.hint', '')
  $('url-input').placeholder = t('ui.urlDialog.placeholder', '')
  $('url-cancel').textContent = t('ui.common.cancel', 'Cancel')
  $('url-ok').textContent = t('ui.common.ok', 'OK')
  $('setting-title').textContent = t('ui.setting.title', '')
  $('setting-close').textContent = t('ui.common.close', t('ui.common.ok', 'Close'))
  $('uigf-title').textContent = t('ui.uigf.title', '')
  $('uigf-cancel').textContent = t('ui.common.cancel', 'Cancel')
  $('uigf-ok').textContent = t('ui.common.ok', 'OK')
}

/* ----------------------------- 弹窗 ----------------------------- */
const openModal = (id) => $(id).classList.add('open')
const closeModal = (id) => $(id).classList.remove('open')

const toast = (msg, type = '') => {
  const el = $('toast')
  el.textContent = msg
  el.className = `toast show ${type}`
  clearTimeout(toast._timer)
  toast._timer = setTimeout(() => { el.className = 'toast' }, 3200)
}

/* --------------------------- 设置界面 --------------------------- */
const renderSettings = () => {
  const s = t('ui.setting', {})
  const cfg = state.config
  const r = state.resources || {}
  const updated = r.checkedAt ? new Date(r.checkedAt).toLocaleString() : '-'
  const body = $('setting-body')
  body.innerHTML = `
    <div class="field">
      <label class="title">${escapeHtml(s.language || '')}</label>
      <select class="select" id="set-lang" style="width:100%">
        ${state.langMap.map(([code, name]) => `<option value="${code}" ${code === cfg.lang ? 'selected' : ''}>${escapeHtml(name)}</option>`).join('')}
      </select>
    </div>
    <div class="field">
      <label class="title">${escapeHtml(t('ui.common.data', 'Data'))}</label>
      <table class="table">
        <thead>
          <tr>
            <th>UID</th>
            <th>${escapeHtml(t('ui.common.updateTime', '更新时间'))}</th>
            <th>${escapeHtml(t('ui.common.status', '状态'))}</th>
            <th>${escapeHtml(t('ui.common.action', '操作'))}</th>
          </tr>
        </thead>
        <tbody id="setting-data-tbody"></tbody>
      </table>
    </div>
    <div class="field field-row">
      <div>
        <label class="title" style="margin:0">${escapeHtml(s.fetchFullHistory || '')}</label>
      </div>
      <label class="switch"><input type="checkbox" id="set-fetchFullHistory" ${cfg.fetchFullHistory ? 'checked' : ''}><span class="slider"></span></label>
    </div>
    <div class="field">
      <label class="title">${escapeHtml(s.resources || '')}</label>
      <div class="resource-box">
        <div>${escapeHtml(s.resourceVersion || '')}：<b>${escapeHtml(r.idVersion || '-')}</b></div>
        <div>${escapeHtml(s.resourceUpdated || '')}：${escapeHtml(updated)}</div>
        <div style="margin-top:6px;display:flex;align-items:center;gap:10px">
          <button class="btn btn-sm" id="set-refresh">${escapeHtml(s.checkResources || '')}</button>
          <span id="set-refresh-msg" style="font-size:12px;color:#64748b"></span>
        </div>
        <div style="margin-top:6px">图标索引：<b>${(state.icons && state.icons.count) || 0}</b> 项</div>
        <div style="margin-top:6px"><button class="btn btn-sm" id="set-clear-icons">清除图标缓存</button></div>
      </div>
    </div>
    <div class="field">
      <label class="title">${escapeHtml(t('ui.about.title', 'About'))}</label>
      <div class="about">
        <div>${escapeHtml(t('ui.about.version', '版本'))}：${state.version ? 'v' + escapeHtml(state.version) : '-'}</div>
        <div>${escapeHtml(t('ui.about.license', ''))}</div>
        <div>核心功能移植自 <a href="https://github.com/earthjasonlin/zzz-signal-search-export" target="_blank" rel="noreferrer">earthjasonlin/zzz-signal-search-export</a></div>
      </div>
    </div>`

  renderDataTable()
  $('set-lang').onchange = async (e) => {
    const res = await api('/api/config', { method: 'POST', body: JSON.stringify({ key: 'lang', value: e.target.value }) })
    if (res.ok) {
      state.i18n = res.i18n
      state.config = res.config
      applyStaticText()
      renderAll()
      renderSettings()
    }
  }
  $('set-fetchFullHistory').onchange = async (e) => {
    const res = await api('/api/config', { method: 'POST', body: JSON.stringify({ key: 'fetchFullHistory', value: e.target.checked }) })
    if (res.ok) state.config = res.config
  }
  $('set-refresh').onclick = async () => {
    const btn = $('set-refresh')
    const msg = $('set-refresh-msg')
    btn.disabled = true
    msg.textContent = t('ui.resource.checking', '')
    const res = await api('/api/resources/refresh', { method: 'POST', body: '{}' })
    btn.disabled = false
    if (res.ok) {
      state.resources = res.resources
      if (res.icons) state.icons = res.icons
      const changed = !!res.resources.updated
      msg.textContent = changed ? t('ui.resource.updated', '') : t('ui.resource.upToDate', '')
      renderSettings()
      toast(msg.textContent, 'success')
    } else {
      msg.textContent = t('ui.resource.failed', '')
      toast(t('ui.resource.failed', ''), 'error')
    }
  }
  $('set-clear-icons').onclick = async () => {
    const res = await api('/api/icons/clear', { method: 'POST', body: '{}' })
    if (res.ok) {
      try { colorCache.clear(); localStorage.removeItem(HISTORY_COLOR_KEY) } catch (e) {}
      state.icons = res.icons
      toast('图标缓存已清除', 'success')
      renderSettings(); renderDetail()
    } else toast('清除失败', 'error')
  }
}

/* --------------------------- 数据管理 --------------------------- */
const renderDataTable = () => {
  const tbody = $('setting-data-tbody')
  const rows = [...state.dataMap.values()]
  if (!rows.length) {
    tbody.innerHTML = `<tr><td colspan="4" style="text-align:center;color:#94a3b8;padding:20px">—</td></tr>`
    return
  }
  tbody.innerHTML = rows.map((d) => `
    <tr>
      <td>${escapeHtml(d.uid)}</td>
      <td>${new Date(d.time).toLocaleString()}</td>
      <td>${d.deleted ? `<span class="tag del">${escapeHtml(t('ui.common.deleted', ''))}</span>` : `<span class="tag ok">${escapeHtml(t('ui.common.normal', ''))}</span>`}</td>
      <td>
        ${!d.deleted && String(d.uid) !== String(state.current) ? `<button class="btn btn-sm" data-switch="${d.uid}">${escapeHtml(t('ui.common.switch', '切换'))}</button>` : ''}
        ${d.deleted
          ? `<button class="btn btn-sm btn-success" data-restore="${d.uid}">${escapeHtml(t('ui.common.restore', ''))}</button>`
          : `<button class="btn btn-sm btn-danger" data-delete="${d.uid}">${escapeHtml(t('ui.common.delete', ''))}</button>`}
      </td>
    </tr>`).join('')

  tbody.querySelectorAll('[data-delete]').forEach((btn) => {
    btn.onclick = () => toggleDelete(btn.getAttribute('data-delete'), true)
  })
  tbody.querySelectorAll('[data-restore]').forEach((btn) => {
    btn.onclick = () => toggleDelete(btn.getAttribute('data-restore'), false)
  })
  tbody.querySelectorAll('[data-switch]').forEach((btn) => {
    btn.onclick = async () => { await changeUid(Number(btn.getAttribute('data-switch'))); renderSettings() }
  })
}

const toggleDelete = async (uid, deleted) => {
  const res = await api('/api/delete', { method: 'POST', body: JSON.stringify({ uid: Number(uid), deleted }) })
  if (res.ok) {
    state.dataMap = hydrateData(res)
    renderDataTable()
    renderAll()
  }
}

/* --------------------------- UIGF 导出 --------------------------- */
const renderUigfModal = () => {
  const entries = visibleEntries()
  $('uigf-list').innerHTML = entries.map((d) => `
    <label class="uid-check"><input type="checkbox" value="${d.uid}" checked> ${escapeHtml(d.uid)}</label>
  `).join('') || `<p class="desc">—</p>`
}

const doExportUigf = async () => {
  const uids = [...$('uigf-list').querySelectorAll('input:checked')].map((i) => Number(i.value))
  if (!uids.length) return toast('请选择 UID', 'error')
  closeModal('modal-uigf')
  toast('UIGF 已导出', 'success')
  window.location.href = `/api/export/uigf?uids=${encodeURIComponent(uids.join(','))}`
}

const doExportExcel = () => {
  if (!currentData()) { toast('暂无数据', 'error'); return }
  toast('Excel 已导出', 'success')
  window.location.href = `/api/export/excel${state.current ? `?uid=${encodeURIComponent(state.current)}` : ''}`
}

/* --------------------------- 抓取数据 --------------------------- */
const doFetchFromGame = async () => {
  if (state.busy) return
  state.busy = true
  state.status = 'loading'
  state.log = '正在从游戏缓存查找抽卡链接…'
  renderAll()
  let res
  try {
    res = await api('/api/fetch-game', { method: 'POST', body: '{}' })
  } catch (e) {
    res = { ok: false, error: String(e.message || e) }
  }
  state.busy = false
  if (res && res.ok) {
    state.dataMap = hydrateData(res)
    state.status = 'loaded'
    state.log = ''
    toast('已从游戏读取数据', 'success')
  } else {
    state.status = 'failed'
    state.log = (res && res.error) || '读取失败'
    toast(state.log, 'error')
  }
  renderAll()
}

const doFetch = async (url) => {
  if (!url || !url.trim()) { toast('请输入 URL', 'error'); return }
  state.busy = true
  state.status = 'loading'
  state.log = ''
  renderAll()
  const res = await api('/api/fetch', { method: 'POST', body: JSON.stringify({ url: url.trim() }) })
  state.busy = false
  if (res.ok) {
    state.dataMap = hydrateData(res)
    state.status = 'loaded'
    state.log = ''
    toast('数据获取成功', 'success')
  } else {
    state.status = 'failed'
    state.log = res.error || 'unknown error'
    toast(res.error || '获取失败', 'error')
  }
  renderAll()
}

/* ----------------------------- 事件 ----------------------------- */
function bindEvents () {
  // 下拉菜单
  document.querySelectorAll('.dropdown > button').forEach((btn) => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation()
      const dd = btn.parentElement
      document.querySelectorAll('.dropdown.open').forEach((x) => { if (x !== dd) x.classList.remove('open') })
      dd.classList.toggle('open')
    })
  })
  document.addEventListener('click', () => {
    document.querySelectorAll('.dropdown.open').forEach((x) => x.classList.remove('open'))
  })
  document.querySelectorAll('.dropdown .menu button').forEach((btn) => {
    btn.addEventListener('click', () => handleCommand(btn.getAttribute('data-cmd')))
  })

  // 关闭按钮
  document.querySelectorAll('[data-close]').forEach((btn) => {
    btn.addEventListener('click', () => closeModal(btn.getAttribute('data-close')))
  })
  document.querySelectorAll('.modal-mask').forEach((mask) => {
    mask.addEventListener('click', (e) => { if (e.target === mask) mask.classList.remove('open') })
  })

  $('btn-url').onclick = () => {
    if (!allowClick() || state.busy) return
    $('url-input').value = ''
    openModal('modal-url')
    setTimeout(() => $('url-input').focus(), 30)
  }
  $('btn-new').onclick = () => { renderSettings(); openModal('modal-setting') }
  $('btn-game').onclick = () => doFetchFromGame()
  $('btn-setting').onclick = () => { renderSettings(); openModal('modal-setting') }
  $('url-ok').onclick = () => {
    const url = $('url-input').value
    closeModal('modal-url')
    doFetch(url)
  }
  $('uid-select').onchange = (e) => changeUid(Number(e.target.value))

  // ⌘V / Ctrl+V：在非输入框状态下直接粘贴链接并获取
  window.addEventListener('paste', (e) => {
    const tag = (e.target && e.target.tagName || '').toLowerCase()
    if (tag === 'input' || tag === 'textarea' || (e.target && e.target.isContentEditable)) return
    const text = (e.clipboardData || window.clipboardData).getData('text') || ''
    const value = text.trim()
    if (/^https?:\/\//i.test(value) || /authkey=/i.test(value)) {
      e.preventDefault()
      doFetch(value)
    }
  })
  $('uigf-ok').onclick = doExportUigf
  $('file-import').onchange = async (e) => {
    const file = e.target.files[0]
    e.target.value = ''
    if (!file) return
    const content = await file.text()
    state.busy = true
    state.status = 'loading'
    state.log = '正在导入 UIGF…'
    renderAll()
    const res = await api('/api/import/uigf', { method: 'POST', body: JSON.stringify({ content }) })
    state.busy = false
    if (res.ok) {
      state.dataMap = hydrateData(res)
      state.status = 'loaded'
      toast('UIGF 导入成功', 'success')
    } else {
      state.status = 'failed'
      state.log = res.error || ''
      toast(res.error || '导入失败', 'error')
    }
    renderAll()
  }
}

const handleCommand = (cmd) => {
  document.querySelectorAll('.dropdown.open').forEach((x) => x.classList.remove('open'))
  if (cmd === 'excel') doExportExcel()
  else if (cmd === 'uigf') { renderUigfModal(); openModal('modal-uigf') }
  else if (cmd === 'import') $('file-import').click()
}

const changeUid = async (uid) => {
  const res = await api('/api/change-uid', { method: 'POST', body: JSON.stringify({ uid }) })
  if (res.ok) {
    state.current = normalizeCurrent(res.current)
    state.status = state.current === 0 ? 'init' : 'loaded'
    renderAll()
  }
}

/* ----------------------------- 启动 ----------------------------- */

const connectEvents = () => {
  try {
    const es = new EventSource('/api/events')
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'STATUS') {
          if (state.status === 'loading') { state.log = msg.text; renderHint() }
        } else if (msg.type === 'ERROR') {
          if (state.status === 'loading') { state.log = msg.text; renderHint() }
        }
      } catch (err) {}
    }
  } catch (e) {}
}

const init = async () => {
  const boot = await api('/api/bootstrap')
  state.version = boot.version
  state.config = boot.config
  state.i18n = boot.i18n
  state.langMap = boot.langMap
  state.resources = boot.resources
  state.icons = boot.icons || {}
  state.dataMap = hydrateData(boot)
  state.status = state.dataMap.get(state.current) ? 'loaded' : 'init'

  applyStaticText()
  bindEvents()
  renderAll()
  if (!location.search.includes('noevents')) connectEvents()

}

init().catch((e) => {
  document.getElementById('hint').textContent = '初始化失败：' + (e.message || e)
})
