<script setup>
import SponsorBar from '../components/SponsorBar.vue'
import { ref, onMounted } from 'vue'

const dataSourceName = ref(typeof window !== 'undefined' ? (window.__DATA_SOURCE__ || 'IPIP 数据库') : 'IPIP 数据库')
const dataSourceUrl = ref(typeof window !== 'undefined' ? (window.__DATA_SOURCE_URL__ || 'https://www.ipip.net') : 'https://www.ipip.net')
const apiBase = ref(typeof window !== 'undefined' ? (window.__API_BASE__ || '/api') : '/api')

onMounted(() => {})
</script>

<template>
  <div class="container mx-auto px-4 py-8">
    <SponsorBar />

    <div class="mb-6 bg-[#1a1a1a] p-4 rounded-lg border border-gray-700 text-sm text-gray-300 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <i class="fas fa-database text-blue-500"></i>
        <span>数据源声明：</span>
        <span class="font-semibold text-blue-400">{{ dataSourceName }}</span>
      </div>
      <a :href="dataSourceUrl" target="_blank" class="text-blue-500 hover:text-blue-400 underline">{{ dataSourceUrl }}</a>
    </div>

    <h2 class="text-2xl font-bold mb-6 flex items-center gap-2 text-white"><i class="fas fa-book text-blue-500"></i> 使用说明</h2>
    <div class="space-y-8">
      <div>
        <h3 class="text-lg font-semibold text-blue-400 mb-2">接口（后端查询）</h3>
        <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
          <div>GET {{ apiBase }}/ip?ip=8.8.8.8</div>
          <div>GET {{ apiBase }}/stats</div>
        </div>
      </div>
      <div>
        <h3 class="text-lg font-semibold text-blue-400 mb-2">查询流程</h3>
        <div class="bg-[#1a1a1a] p-4 rounded-lg font-mono text-sm border border-gray-700 text-gray-300">
          <div>1) 前端发起 /api/ip 请求</div>
          <div>2) 后端解析 IP 并命中本地缓存或数据库</div>
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
</template>

