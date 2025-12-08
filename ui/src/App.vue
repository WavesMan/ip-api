<script setup>
import { ref } from 'vue'

const queryIp = ref('')
const searchResult = ref(null)
const error = ref('')
const loading = ref(false)

async function onSearch() {
  if (!queryIp.value) return
  loading.value = true
  error.value = ''
  searchResult.value = null
  try {
    const url = `/api/ip?ip=${encodeURIComponent(queryIp.value)}`
    const res = await fetch(url)
    if (!res.ok) throw new Error('查询失败')
    const data = await res.json()
    searchResult.value = data
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
          基于 Go 后端与数据库的 IP 归属地查询服务
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
            <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.region || '-' }}</span><span class="text-gray-500">区域</span></div>
            <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.province || '-' }}</span><span class="text-gray-500">省份</span></div>
            <div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ searchResult.city || '-' }}</span><span class="text-gray-500">城市</span></div>
            <div class="flex justify-between"><span class="font-medium">{{ searchResult.isp || '-' }}</span><span class="text-gray-500">运营商</span></div>
          </div>
          <div class="flex justify-between items-center pt-4 border-t border-gray-700 mt-auto text-sm text-gray-500 relative z-10"><div class="flex items-center gap-2"><i class="fas fa-check-circle text-green-500"></i><span>查询成功</span></div></div>
        </div>
        
      </div>

      
      <div class="bg-[#2d2d2d] rounded-xl p-8 border border-gray-700">
        <h2 class="text-2xl font-bold mb-6 flex items-center gap-2 text-white"><i class="fas fa-book text-blue-500"></i> 使用说明</h2>
        <div class="space-y-8">
          <div>
            <h3 class="text-lg font-semibold text-blue-400 mb-2">接口（后端查询）</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
              <div>GET /api/ip?ip=8.8.8.8</div>
              <div>GET /api/stats</div>
            </div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-blue-400 mb-2">查询流程</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
              <div>1) 前端发起 /api/ip 请求</div>
              <div>2) 后端解析 IP 并命中数据库</div>
              <div>3) 命中则返回字典字段（国家/区域/省/市/运营商）</div>
              <div>4) 记录统计并缓存结果</div>
            </div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-purple-400 mb-2">部署说明</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
              <div>后端服务静态托管 ui/dist</div>
              <div>数据库：PostgreSQL，缓存：Redis</div>
            </div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-green-400 mb-2">示例响应</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300"><pre>{
  "ip": "8.8.8.8",
  "country": "美国",
  "region": "北美",
  "province": "加利福尼亚",
  "city": "山景城",
  "isp": "Google LLC"
}</pre></div>
          </div>
          <div>
            <h3 class="text-lg font-semibold text-green-400 mb-2">统计响应示例</h3>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300"><pre>{
  "total": 12345,
  "today": 67
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
