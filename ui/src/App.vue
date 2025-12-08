<script setup>
import { ref, onMounted } from 'vue'
import DocsPage from './pages/DocsPage.vue'
const activePage = ref('home')
function goto(page) { activePage.value = page }

const queryIp = ref('')
const searchResult = ref(null)
const error = ref('')
const loading = ref(false)

const stats = ref({ total: 0, today: 0 })
const statsLoading = ref(false)
const statsError = ref('')

const showResult = ref(false)

async function fetchStats() {
  statsLoading.value = true
  statsError.value = ''
  try {
    const base = window.__API_BASE__ || '/api'
    const res = await fetch(`${base}/stats`)
    if (!res.ok) throw new Error('统计获取失败')
    const data = await res.json()
    stats.value = data || { total: 0, today: 0 }
  } catch (e) {
    statsError.value = e.message || '统计获取失败'
  } finally {
    statsLoading.value = false
  }
}

onMounted(() => {
  fetchStats()
})

const visitorResult = ref(null)
async function fetchVisitor() {
  try {
    const base = window.__API_BASE__ || '/api'
    const res = await fetch(`${base}/ip`)
    if (!res.ok) return
    const data = await res.json()
    visitorResult.value = data
  } catch {}
}

onMounted(() => {
  fetchVisitor()
})

const dataSourceName = ref(typeof window !== 'undefined' ? (window.__DATA_SOURCE__ || 'MaxMind GeoIP') : 'MaxMind GeoIP')
const dataSourceUrl = ref(typeof window !== 'undefined' ? (window.__DATA_SOURCE_URL__ || 'https://www.maxmind.com') : 'https://www.maxmind.com')

const commitSha = ref(typeof window !== 'undefined' ? (window.__COMMIT_SHA__ || 'dev') : 'dev')
const builtAt = ref('')
async function fetchVersion() {
  try {
    const base = window.__API_BASE__ || '/api'
    const res = await fetch(`${base}/version`)
    if (!res.ok) return
    const data = await res.json()
    if (data && data.commit) commitSha.value = data.commit
    if (data && data.builtAt) builtAt.value = data.builtAt
  } catch {}
}
onMounted(() => { fetchVersion() })

async function onSearch() {
  if (!queryIp.value) return
  loading.value = true
  error.value = ''
  showResult.value = true
  try {
    const base = window.__API_BASE__ || '/api'
    const url = `${base}/ip?ip=${encodeURIComponent(queryIp.value)}`
    const res = await fetch(url)
    if (!res.ok) throw new Error('查询失败')
    const data = await res.json()
    searchResult.value = data
    showResult.value = true
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
            <li><a href="#" @click.prevent="goto('home')" :class="activePage==='home'?'text-blue-500':''" class="hover:text-blue-500 transition-colors">首页</a></li>
            <li><a href="#" @click.prevent="goto('docs')" :class="activePage==='docs'?'text-blue-500':''" class="hover:text-blue-500 transition-colors">接口说明</a></li>
            <li><a href="https://home.waveyo.cn" target="_blank" class="hover:text-blue-500 transition-colors">WaveYo Home</a></li>
            <li><a href="https://blog.waveyo.cn" target="_blank" class="hover:text-blue-500 transition-colors">WaveYo Blog</a></li>
            <li><a href="https://github.com/WavesMan" target="_blank" class="hover:text-blue-500 transition-colors">GitHub</a></li>
          </ul>
        </nav>
      </div>
    </header>

    
    <main v-if="activePage==='home'" class="flex-grow container mx-auto px-4 py-8">
      
      <div class="text-center mb-12">
        <h1 class="text-4xl md:text-5xl font-bold mb-4 bg-gradient-to-r from-blue-500 to-purple-600 bg-clip-text text-transparent">
          极速、精准
        </h1>
        <p class="text-xl text-gray-400 mb-8">
          基于 Go 后端与数据库的 IP 归属地查询服务
        </p>

        <div class="flex flex-wrap justify-center gap-3 mb-6">
          <div class="bg-[#2d2d2d] rounded-xl px-6 py-4 border border-gray-700 flex items-center gap-4">
            <div class="text-gray-400">
              <div class="text-sm">总计服务量</div>
              <div class="text-2xl font-bold text-blue-400">{{ stats.total }}</div>
            </div>
            <div class="w-px h-10 bg-gray-700"></div>
            <div class="text-gray-400">
              <div class="text-sm">今日服务量</div>
              <div class="text-2xl font-bold text-green-400">{{ stats.today }}</div>
            </div>
            <button @click="fetchStats" :disabled="statsLoading" class="ml-4 text-sm px-3 py-1 rounded bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50">刷新</button>
          </div>
          <div v-if="statsError" class="text-red-500 text-sm flex items-center"><i class="fas fa-exclamation-circle mr-1"></i>{{ statsError }}</div>
        </div>
        <div v-if="visitorResult" class="bg-[#2d2d2d] rounded-xl px-6 py-4 border border-gray-700 flex items-center gap-6 w-full max-w-4xl mx-auto mt-6 mb-8">
          <div class="flex-1 min-w-0 text-gray-300 flex items-center gap-4">
            <div class="shrink-0 text-sm text-gray-400">你的来访 IP</div>
            <div class="text-2xl font-bold text-purple-400 truncate">{{ visitorResult.ip }}</div>
          </div>
          <div class="flex-1 min-w-0 text-gray-300 flex items-center gap-4">
            <div class="shrink-0 text-sm text-gray-400">属地</div>
            <div class="font-semibold truncate">{{ [visitorResult.country, visitorResult.region, visitorResult.province, visitorResult.city].filter(Boolean).join(' / ') || '-' }}</div>
          </div>
          <div class="flex-1 min-w-0 text-gray-300 flex items-center gap-4">
            <div class="shrink-0 text-sm text-gray-400">运营商</div>
            <div class="font-semibold truncate">{{ visitorResult.isp || '-' }}</div>
          </div>
        </div>
        
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
        <div v-show="showResult" class="bg-[#2d2d2d] rounded-xl p-6 transition-all border border-blue-500/30 flex flex-col relative overflow-hidden w-full max-w-xl min-h-[260px]">
          <div class="absolute top-0 right-0 p-2 opacity-10"><i class="fas fa-search text-6xl"></i></div>
          <div class="flex justify-between items-start mb-4 relative z-10">
            <div class="flex-1 min-w-0">
              <h3 class="text-xl font-bold" :class="loading ? 'text-blue-400' : 'text-green-400'">{{ loading ? '查询中…' : '查询结果' }}</h3>
              <p class="text-xs text-gray-500 font-mono mt-1 truncate">{{ (searchResult && searchResult.ip) || (loading ? queryIp : '') }}</p>
            </div>
            <div class="bg-gray-800 text-xs px-2 py-1 rounded text-gray-400 whitespace-nowrap ml-2">Result</div>
          </div>
          <div class="space-y-2 mb-6 flex-grow relative z-10">
            <template v-if="!loading && searchResult">
              <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.country || '-' }}</span><span class="text-gray-500">国家</span></div>
              <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.region || '-' }}</span><span class="text-gray-500">区域</span></div>
              <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.province || '-' }}</span><span class="text-gray-500">省份</span></div>
              <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.city || '-' }}</span><span class="text-gray-500">城市</span></div>
              <div class="flex justify-between"><span class="font-medium">{{ searchResult.isp || '-' }}</span><span class="text-gray-500">运营商</span></div>
            </template>
            <template v-else>
              <div class="animate-pulse space-y-3">
                <div class="h-4 bg-gray-700 rounded"></div>
                <div class="h-4 bg-gray-700 rounded"></div>
                <div class="h-4 bg-gray-700 rounded"></div>
                <div class="h-4 bg-gray-700 rounded"></div>
                <div class="h-4 bg-gray-700 rounded"></div>
              </div>
            </template>
          </div>
          <div class="flex justify-between items-center pt-4 border-t border-gray-700 mt-auto text-sm text-gray-500 relative z-10">
            <div class="flex items-center gap-2">
              <i :class="loading ? 'fas fa-spinner fa-spin text-blue-500' : 'fas fa-check-circle text-green-500'"></i>
              <span>{{ loading ? '查询中…' : '查询成功' }}</span>
            </div>
          </div>
        </div>
        
      </div>

      
    <div class="interface-card bg-[#2d2d2d] rounded-xl p-8 border border-gray-700 max-w-3xl mx-auto">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-2xl font-bold flex items-center gap-2 text-white">
          <i class="fas fa-book text-blue-500"></i>
          接口说明
        </h2>
        <button 
          @click="goto('docs')" 
          class="px-4 py-2 rounded bg-blue-600 hover:bg-blue-700 text-white transition-colors duration-200"
        >
          查看接口说明
        </button>
      </div>
      <p class="text-gray-400">接口使用说明已迁移至专门页面，点击右侧按钮查看。</p>
    </div>

    </main>
    <DocsPage v-if="activePage==='docs'" />

    
    <footer class="border-t border-gray-700 py-8 text-gray-500">
      <div class="container mx-auto px-4">
        <div class="flex justify-between items-center">
          <p>&copy; {{ new Date().getFullYear() }} WaveYo.  
            <a href="https://www.rainyun.com/WaveYo_" target="_blank" class="hover:text-blue-400 transition-colors duration-200">雨云提供计算支持</a> | 
            EdgeOne提供CDN支持
          </p>
          <p>
            <a href="https://beian.miit.gov.cn/" target="_blank" class="hover:text-blue-400 transition-colors duration-200">皖ICP备2025078205号</a>&nbsp;&nbsp;
            <a href="https://beian.mps.gov.cn/#/query/webSearch?code=" rel="noreferrer" target="_blank" class="hover:text-blue-400 transition-colors duration-200">皖公网安备34040002000514号</a>
          </p>
        </div>
        <p class="mt-2 text-xs text-gray-400 font-mono">Commit: {{ commitSha }} <span v-if="builtAt">| BuiltAt: {{ builtAt }}</span></p>
      </div>
    </footer>
  </div>
</template>

<style scoped>
</style>
