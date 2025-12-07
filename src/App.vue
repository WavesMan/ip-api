<script setup>
import { ref } from 'vue'

const queryIp = ref('')
const searchResult = ref(null)
const error = ref('')
const loading = ref(false)

let dictLoaded = false
let strings = null
let triples = null
const chunkCache = new Map()

function ipToInt(ip) {
  const p = ip.split('.')
  if (p.length !== 4) return null
  for (let i = 0; i < 4; i++) {
    const n = Number(p[i])
    if (!Number.isInteger(n) || n < 0 || n > 255) return null
  }
  return ((Number(p[0]) << 24) >>> 0) + (Number(p[1]) << 16) + (Number(p[2]) << 8) + Number(p[3])
}

async function fetchBin(url) {
  const res = await fetch(url)
  if (!res.ok) throw new Error('资源加载失败')
  return new Uint8Array(await res.arrayBuffer())
}

async function ensureDict() {
  if (dictLoaded) return
  const DB = await fetchBin('/db/dict.bin')
  const dv = new DataView(DB.buffer, DB.byteOffset, DB.byteLength)
  let off = 0
  const m0 = dv.getUint8(off++)
  const m1 = dv.getUint8(off++)
  const m2 = dv.getUint8(off++)
  const m3 = dv.getUint8(off++)
  if (!(m0 === 0x49 && m1 === 0x50 && m2 === 0x44 && m3 === 0x43)) {
    strings = []
    triples = []
    dictLoaded = true
    return
  }
  const ver = dv.getUint8(off++)
  const strCount = dv.getUint32(off, true); off += 4
  const triCount = dv.getUint32(off, true); off += 4
  strings = new Array(strCount)
  const td = new TextDecoder()
  for (let i = 0; i < strCount; i++) {
    const len = dv.getUint16(off, true); off += 2
    const bytes = DB.subarray(off, off + len); off += len
    strings[i] = td.decode(bytes)
  }
  triples = new Array(triCount)
  for (let i = 0; i < triCount; i++) {
    const a = dv.getUint16(off, true); off += 2
    const b = dv.getUint16(off, true); off += 2
    const c = dv.getUint16(off, true); off += 2
    triples[i] = [a, b, c]
  }
  dictLoaded = true
}

async function loadChunk(a) {
  const cached = chunkCache.get(a)
  if (cached) return cached
  const DB = await fetchBin(`/db/chunks/a${a}.bin`)
  const dv = new DataView(DB.buffer, DB.byteOffset, DB.byteLength)
  let off = 0
  const m0 = dv.getUint8(off++)
  const m1 = dv.getUint8(off++)
  const m2 = dv.getUint8(off++)
  const m3 = dv.getUint8(off++)
  if (!(m0 === 0x49 && m1 === 0x50 && m2 === 0x43 && m3 === 0x48)) {
    const arr = []
    chunkCache.set(a, arr)
    return arr
  }
  const ver = dv.getUint8(off++)
  const recCount = dv.getUint32(off, true); off += 4
  const readVar = () => {
    let x = 0
    let shift = 0
    while (true) {
      const b = dv.getUint8(off++)
      x |= (b & 0x7f) << shift
      if ((b & 0x80) === 0) break
      shift += 7
    }
    return x >>> 0
  }
  const records = new Array(recCount)
  let prevStart = 0
  for (let i = 0; i < recCount; i++) {
    const delta = readVar()
    const start = (prevStart + delta) >>> 0
    const len = readVar()
    const end = (start + len) >>> 0
    const t = dv.getUint16(off, true); off += 2
    records[i] = [start, end, t]
    prevStart = start
  }
  chunkCache.set(a, records)
  return records
}

async function lookup(ip) {
  await ensureDict()
  const val = ipToInt(ip)
  if (val == null) return { country: null, province: null, city: null }
  const a = (val >>> 24) & 0xff
  const records = await loadChunk(a)
  let lo = 0
  let hi = records.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1
    const r = records[mid]
    if (val < r[0]) hi = mid - 1; else if (val > r[1]) lo = mid + 1; else {
      const tri = triples[r[2]] || [0, 0, 0]
      const country = strings[tri[0]] || null
      const province = strings[tri[1]] || null
      const city = strings[tri[2]] || null
      return { country, province, city }
    }
  }
  return { country: null, province: null, city: null }
}

async function onSearch() {
  if (!queryIp.value) return
  loading.value = true
  error.value = ''
  searchResult.value = null
  try {
    const res = await lookup(queryIp.value)
    searchResult.value = { ip: queryIp.value, ...res }
  } catch (e) {
    error.value = e.message || '查询失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex flex-col bg-[#1a1a1a] text-[#e5e7eb] font-sans">
    
    <header class="border-b border-gray-700 bg-[#1a1a1a] sticky top-0 z-50">
      <div class="container mx-auto px-4 py-4 flex justify-between items-center">
        <div class="text-2xl font-bold text-blue-500 flex items-center gap-2">
          <i class="fas fa-globe-asia"></i>
          <span>IP 归属地查询</span>
        </div>
        <nav>
          <ul class="flex gap-6 text-gray-300">
            <li><a href="https://home.waveyo.cn" target="_blank" class="hover:text-blue-500 transition-colors">WaveYo Home</a></li>
            <li><a href="https://blog.waveyo.cn" target="_blank" class="hover:text-blue-500 transition-colors">WaveYo Blog</a></li>
            <li><a href="https://github.com/WavesMan/ip-api" target="_blank" class="hover:text-blue-500 transition-colors">GitHub</a></li>
          </ul>
        </nav>
      </div>
    </header>

    
    <main class="flex-grow container mx-auto px-4 py-8">
      
      <div class="text-center mb-12">
        <h1 class="text-4xl md:text-5xl font-bold mb-4 bg-gradient-to-r from-blue-500 to-purple-600 bg-clip-text text-transparent">
          极速、精准、离线
        </h1>
        <p class="text-xl text-gray-400 mb-8">
          基于边缘函数构建的 IP 归属地查询服务
        </p>

        <div class="flex flex-wrap justify-center gap-3 mb-6"></div>
        
        <div class="max-w-2xl mx-auto relative">
          <input 
            v-model="queryIp"
            @keyup.enter="onSearch"
            type="text" 
            placeholder="输入 IPv4 地址查询，例如 8.8.8.8" 
            class="w-full px-6 py-4 rounded-full bg-[#2d2d2d] border border-gray-700 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none text-lg transition-all"
          >
          <button 
            @click="onSearch"
            :disabled="loading"
            class="absolute right-2 top-1/2 -translate-y-1/2 px-6 py-2 bg-blue-600 hover:bg-blue-700 rounded-full text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <i class="fas" :class="loading ? 'fa-spinner fa-spin' : 'fa-search'"></i>
            <span class="ml-2 hidden md:inline">{{ loading ? '查询中...' : '查询' }}</span>
          </button>
        </div>
        <div v-if="error" class="mt-4 text-red-500">
          <i class="fas fa-exclamation-circle mr-1"></i> {{ error }}
        </div>
      </div>

      <div class="flex justify-center mb-12">
        <div v-if="searchResult" class="bg-[#2d2d2d] rounded-xl p-6 hover:-translate-y-1 hover:shadow-xl hover:shadow-black/30 transition-all border border-blue-500/30 hover:border-blue-500 flex flex-col relative overflow-hidden w-full max-w-xl">
          <div class="absolute top-0 right-0 p-2 opacity-10"><i class="fas fa-search text-6xl"></i></div>
          <div class="flex justify-between items-start mb-4 relative z-10">
            <div class="flex-1 min-w-0"><h3 class="text-xl font-bold text-green-400 truncate">查询结果</h3><p class="text-xs text-gray-500 font-mono mt-1 truncate">{{ searchResult.ip }}</p></div>
            <div class="bg-gray-800 text-xs px-2 py-1 rounded text-gray-400 whitespace-nowrap ml-2">Result</div>
          </div>
          <div class="space-y-2 mb-6 flex-grow relative z-10">
            <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.country || '-' }}</span><span class="text-gray-500">国家</span></div>
            <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.province || '-' }}</span><span class="text-gray-500">省份</span></div>
            <div class="flex justify-between"><span class="font-medium">{{ searchResult.city || '-' }}</span><span class="text-gray-500">城市</span></div>
          </div>
          <div class="flex justify-between items-center pt-4 border-t border-gray-700 mt-auto text-sm text-gray-500 relative z-10"><div class="flex items-center gap-2"><i class="fas fa-check-circle text-green-500"></i><span>查询成功</span></div></div>
        </div>
        
      </div>

      
      <div class="bg-[#2d2d2d] rounded-xl p-8 border border-gray-700">
        <h2 class="text-2xl font-bold mb-6 flex items-center gap-2 text-white"><i class="fas fa-book text-blue-500"></i> 使用说明</h2>
        <div class="space-y-8">
          <div>
            <h3 class="text-lg font-semibold text-blue-400 mb-2">静态资源（前端本地查询）</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
              <div>/db/dict.bin</div>
              <div>/db/chunks/a{0..255}.bin</div>
            </div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-blue-400 mb-2">查询流程</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
              <div>1) 将 IPv4 转整型</div>
              <div>2) 加载 dict.bin</div>
              <div>3) 按首段加载 a{X}.bin</div>
              <div>4) 二分查找命中区间，还原三元组</div>
            </div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-purple-400 mb-2">外部接口（Workers）</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
              <div>GET /api/ip?ip=8.8.8.8</div>
              <div>响应头：content-type: application/json; charset=utf-8</div>
            </div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-green-400 mb-2">示例响应</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300"><pre>{
  "ip": "8.8.8.8",
  "country": "美国",
  "province": "加利福尼亚",
  "city": "山景城"
}</pre></div>
          </div>
        </div>
      </div>

    </main>

    
    <footer class="border-t border-gray-700 py-8 text-center text-gray-500">
      <p>&copy; {{ new Date().getFullYear() }} WaveYo. Powered by EdgeOne Pages & Vue 3.</p>
    </footer>
  </div>
</template>

<style scoped>
/* Scoped styles if needed, mostly utilizing Tailwind classes */
</style>
