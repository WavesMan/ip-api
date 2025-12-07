<script setup>
import { ref, onMounted, computed } from 'vue'

const visitor = ref(null)
const queryIp = ref('')
const searchResult = ref(null)
const stats = ref({ total: 0, today: 0 })
const error = ref('')
const loading = ref(false)
const copied = ref(false)

const endpoint = computed(() => `${window.location.origin}/api/ip`)

async function fetchVisitor() {
  try {
    const res = await fetch('/api/ip')
    if (res.ok) visitor.value = await res.json()
  } catch (e) {
    console.error(e)
  }
}

async function onSearch() {
  if (!queryIp.value) return
  loading.value = true
  error.value = ''
  searchResult.value = null
  try {
    const res = await fetch(`/api/ip?ip=${queryIp.value}`)
    if (!res.ok) throw new Error('查询失败')
    searchResult.value = await res.json()
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function fetchStats() {
  try {
    const res = await fetch('/api/stats')
    if (res.ok) stats.value = await res.json()
  } catch (e) {
    console.error(e)
  }
}

async function copyEndpoint() {
  try {
    await navigator.clipboard.writeText(endpoint.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {}
}

onMounted(() => {
  fetchVisitor()
  fetchStats()
})
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

        <div class="flex flex-wrap justify-center gap-3 mb-6">
          <div class="px-3 py-2 rounded bg-[#2d2d2d] border border-gray-700 text-sm text-gray-300">
            <span class="text-gray-500">累计服务</span>
            <span class="ml-2 font-mono font-bold">{{ stats.total }}</span>
          </div>
          <div class="px-3 py-2 rounded bg-[#2d2d2d] border border-gray-700 text-sm text-gray-300">
            <span class="text-gray-500">今日调用</span>
            <span class="ml-2 font-mono font-bold">{{ stats.today }}</span>
          </div>
          <div class="px-3 py-2 rounded bg-[#2d2d2d] border border-gray-700 text-sm text-gray-300">
             <span class="text-gray-500">API 端点：</span>
             <div class="mt-1 inline-flex items-center gap-2">
               <span class="font-mono break-all">{{ endpoint }}</span>
               <button @click="copyEndpoint" class="px-2 py-1 bg-gray-800 hover:bg-blue-600 rounded text-xs border border-gray-700 transition-colors flex items-center gap-1 ml-1">
                 <i class="fas fa-copy"></i>
                 <span v-if="!copied">复制</span>
                 <span v-else>已复制</span>
               </button>
             </div>
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
        <div v-else-if="visitor" class="bg-[#2d2d2d] rounded-xl p-6 hover:-translate-y-1 hover:shadow-xl hover:shadow-black/30 transition-all border border-transparent hover:border-gray-700 flex flex-col w-full max-w-xl">
          <div class="flex justify-between items-start mb-4"><div class="flex-1 min-w-0"><h3 class="text-xl font-bold text-blue-400 truncate">您的当前 IP</h3><p class="text-xs text-gray-500 font-mono mt-1 truncate">{{ visitor.ip }}</p></div><div class="bg-gray-800 text-xs px-2 py-1 rounded text-gray-400 whitespace-nowrap ml-2">Current</div></div>
          <div class="space-y-2 mb-6 flex-grow"><div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ visitor.country || '-' }}</span><span class="text-gray-500">国家</span></div><div class="flex justify-between border-b border-gray-700/50 pb-2"><span class="font-medium">{{ visitor.province || '-' }}</span><span class="text-gray-500">省份</span></div><div class="flex justify-between"><span class="font-medium">{{ visitor.city || '-' }}</span><span class="text-gray-500">城市</span></div></div>
          <div class="flex justify-between items-center pt-4 border-t border-gray-700 mt-auto text-sm text-gray-500"><div class="flex items-center gap-2"><i class="fas fa-map-marker-alt"></i><span>自动检测</span></div></div>
        </div>
      </div>

      
      <div class="bg-[#2d2d2d] rounded-xl p-8 border border-gray-700">
        <h2 class="text-2xl font-bold mb-6 flex items-center gap-2 text-white">
          <i class="fas fa-book text-blue-500"></i> API 使用说明
        </h2>
        
        <div class="space-y-8">
          <div>
            <h3 class="text-lg font-semibold text-blue-400 mb-2">1. 基础 IP 查询</h3>
            <p class="text-gray-400 mb-3">查询指定 IPv4 地址的归属地信息。</p>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700">
              <div class="flex gap-2 mb-2">
                <span class="text-purple-400">GET</span>
                <span class="text-gray-300">/api/ip?ip={ip_address}</span>
              </div>
              <div class="text-gray-500">// 示例：查询 8.8.8.8</div>
              <div class="text-blue-300 mb-2">curl "{{ endpoint }}?ip=8.8.8.8"</div>
            </div>
          </div>

          <div>
            <h3 class="text-lg font-semibold text-blue-400 mb-2">2. 访客 IP 识别</h3>
            <p class="text-gray-400 mb-3">不带参数调用接口，自动返回调用者的 IP 归属地。</p>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700">
              <div class="flex gap-2 mb-2">
                <span class="text-purple-400">GET</span>
                <span class="text-gray-300">/api/ip</span>
              </div>
              <div class="text-blue-300 mb-2">curl "{{ endpoint }}"</div>
            </div>
          </div>

          <div>
            <h3 class="text-lg font-semibold text-green-400 mb-2">响应格式</h3>
            <p class="text-gray-400 mb-3">所有接口均返回标准 JSON 格式数据。</p>
            <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
<pre>{
  "ip": "8.8.8.8",
  "country": "美国",
  "province": "加利福尼亚",
  "city": "山景城"
}</pre>
            </div>
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
