<script setup>
import { ref, onMounted } from 'vue'

const emit = defineEmits([])
const isMobile = ref(false)

onMounted(() => {
  const ua = navigator.userAgent || ''
  isMobile.value = /Mobile|Android|iPhone|iPad/i.test(ua) || window.innerWidth < 640
})

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

onMounted(() => { fetchStats() })

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

onMounted(() => { fetchVisitor() })

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
  <main class="home">
    <div class="hero">
      <h1 class="hero__title">极速、精准</h1>
      <p class="hero__desc">基于 Go 后端与数据库的 IP 归属地查询服务</p>

      <div class="stats">
        <div class="stats__card">
          <div class="stats__section">
            <div class="stats__label">总计服务量</div>
            <div class="stats__value stats__value--blue">{{ stats.total }}</div>
          </div>
          <div class="stats__divider"></div>
          <div class="stats__section">
            <div class="stats__label">今日服务量</div>
            <div class="stats__value stats__value--green">{{ stats.today }}</div>
          </div>
          <button @click="fetchStats" :disabled="statsLoading" class="stats__refresh">刷新</button>
        </div>
        <div v-if="statsError" class="error"><i class="fas fa-exclamation-circle mr-1"></i>{{ statsError }}</div>
      </div>

      <div v-if="visitorResult" class="visitor" :class="isMobile ? 'visitor--mobile' : 'visitor--desktop'">
        <div class="visitor__section">
          <div class="visitor__label">你的来访 IP</div>
          <div class="visitor__value visitor__value--ip">{{ visitorResult.ip }}</div>
        </div>
        <div class="visitor__section">
          <div class="visitor__label">属地</div>
          <div class="visitor__value visitor__value--text">{{ [visitorResult.country, visitorResult.region, visitorResult.province, visitorResult.city].filter(Boolean).join(' / ') || '-' }}</div>
        </div>
        <div class="visitor__section">
          <div class="visitor__label">运营商</div>
          <div class="visitor__value visitor__value--text">{{ visitorResult.isp || '-' }}</div>
        </div>
      </div>

      <div class="search">
        <input v-model="queryIp" @keyup.enter="onSearch" type="text" placeholder="输入 IPv4 地址查询，例如 8.8.8.8" class="search__input">
        <button @click="onSearch" :disabled="loading" class="search__btn">
          <i class="fas" :class="loading ? 'fa-spinner fa-spin' : 'fa-search'"></i>
          <span class="search__btn-text">{{ loading ? '查询中...' : '查询' }}</span>
        </button>
      </div>
      <div v-if="error" class="error"><i class="fas fa-exclamation-circle mr-1"></i> {{ error }}</div>
    </div>

    <div class="result-wrap">
      <div v-show="showResult" class="result">
        <div class="result__badge"><i class="fas fa-search result__badge-icon"></i></div>
        <div class="result__header">
          <div class="result__header-left">
            <h3 class="result__title" :class="loading ? 'result__title--loading' : 'result__title--ok'">{{ loading ? '数据库数据不完整，正在分析…' : '查询结果' }}</h3>
            <p class="result__ip">{{ (searchResult && searchResult.ip) || (loading ? queryIp : '') }}</p>
          </div>
          <div class="result__header-right">Result</div>
        </div>
        <div class="result__content">
          <template v-if="!loading && searchResult">
            <div class="result__item"><span class="result__item-value">{{ searchResult.country || '-' }}</span><span class="result__item-label">国家</span></div>
            <div class="result__item"><span class="result__item-value">{{ searchResult.region || '-' }}</span><span class="result__item-label">区域</span></div>
            <div class="result__item"><span class="result__item-value">{{ searchResult.province || '-' }}</span><span class="result__item-label">省份</span></div>
            <div class="result__item"><span class="result__item-value">{{ searchResult.city || '-' }}</span><span class="result__item-label">城市</span></div>
            <div class="result__item"><span class="result__item-value">{{ searchResult.isp || '-' }}</span><span class="result__item-label">运营商</span></div>
          </template>
          <template v-else>
            <div class="skeleton">
              <div class="skeleton__line"></div>
              <div class="skeleton__line"></div>
              <div class="skeleton__line"></div>
              <div class="skeleton__line"></div>
              <div class="skeleton__line"></div>
            </div>
          </template>
        </div>
        <div class="result__footer">
          <div class="result__status">
            <i :class="loading ? 'fas fa-spinner fa-spin text-blue-500' : 'fas fa-check-circle text-green-500'"></i>
            <span>{{ loading ? '数据库数据不完整，正在分析…' : '查询成功' }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="iface">
      <div class="iface__header">
        <h2 class="iface__title">
          <i class="fas fa-book iface__icon"></i>
          接口说明
        </h2>
        <RouterLink to="/docs" class="iface__btn">查看接口说明</RouterLink>
      </div>
      <p class="iface__desc">接口使用说明已迁移至专门页面，点击右侧按钮查看。</p>
    </div>
  </main>
</template>

<style scoped>
@reference "../style.css";
.home { 
  @apply flex-grow; 
  @apply container; 
  @apply mx-auto; 
  @apply px-4; 
  @apply py-8; 
}

.hero { 
  @apply text-center; 
  @apply mb-12; 
}

.hero__title { 
  @apply text-4xl; 
  @apply md:text-5xl; 
  @apply font-bold; 
  @apply mb-4; 
  @apply bg-gradient-to-r; 
  @apply from-blue-500; 
  @apply to-purple-600; 
  @apply bg-clip-text; 
  @apply text-transparent; 
}

.hero__desc { 
  @apply text-xl; 
  @apply text-gray-400; 
  @apply mb-8; 
}

.stats { 
  @apply flex; 
  @apply flex-wrap; 
  @apply justify-center; 
  @apply gap-3; 
  @apply mb-6; 
}

.stats__card { 
  @apply bg-[#2d2d2d]; 
  @apply rounded-xl; 
  @apply px-6; 
  @apply py-4; 
  @apply border; 
  @apply border-gray-700; 
  @apply flex; 
  @apply items-center; 
  @apply gap-4; 
}

.stats__section { 
  @apply text-gray-400; 
}

.stats__label { 
  @apply text-sm; 
}

.stats__value { 
  @apply text-2xl; 
  @apply font-bold; 
}

.stats__value--blue { 
  @apply text-blue-400; 
}

.stats__value--green { 
  @apply text-green-400; 
}

.stats__divider { 
  @apply w-px; 
  @apply h-10; 
  @apply bg-gray-700; 
}

.stats__refresh { 
  @apply ml-4; 
  @apply text-sm; 
  @apply px-3; 
  @apply py-1; 
  @apply rounded; 
  @apply bg-blue-600; 
  @apply hover:bg-blue-700; 
  @apply text-white; 
  @apply disabled:opacity-50; 
}

.visitor { 
  @apply bg-[#2d2d2d]; 
  @apply rounded-xl; 
  @apply px-6; 
  @apply py-4; 
  @apply border; 
  @apply border-gray-700; 
  @apply flex; 
  @apply w-full; 
  @apply max-w-4xl; 
  @apply mx-auto; 
  @apply mt-6; 
  @apply mb-8; 
}

.visitor--mobile { 
  @apply flex-col; 
  @apply items-start; 
  @apply gap-4; 
}

.visitor--desktop { 
  @apply items-center; 
  @apply gap-6; 
}

.visitor__section { 
  @apply flex-1; 
  @apply min-w-0; 
  @apply text-gray-300; 
  @apply flex; 
  @apply items-center; 
  @apply gap-4; 
}

.visitor__label { 
  @apply shrink-0; 
  @apply text-sm; 
  @apply text-gray-400; 
}

.visitor__value { 
  @apply font-semibold; 
  @apply truncate; 
}

.visitor__value--ip { 
  @apply text-2xl; 
  @apply font-bold; 
  @apply text-purple-400; 
}

.search { 
  @apply max-w-2xl; 
  @apply mx-auto; 
  @apply relative; 
}

.search__input { 
  @apply w-full; 
  @apply px-6; 
  @apply py-4; 
  @apply rounded-full; 
  @apply bg-[#2d2d2d]; 
  @apply border; 
  @apply border-gray-700; 
  @apply focus:border-blue-500; 
  @apply focus:ring-2; 
  @apply focus:ring-blue-500/20; 
  @apply outline-none; 
  @apply text-lg; 
  @apply transition-all; 
}

.search__btn { 
  @apply absolute; 
  @apply right-2; 
  @apply top-1/2; 
  @apply -translate-y-1/2; 
  @apply px-6; 
  @apply py-2; 
  @apply bg-blue-600; 
  @apply hover:bg-blue-700; 
  @apply rounded-full; 
  @apply text-white; 
  @apply font-medium; 
  @apply transition-colors; 
  @apply disabled:opacity-50; 
  @apply disabled:cursor-not-allowed; 
}

.search__btn-text { 
  @apply ml-2; 
  @apply hidden; 
  @apply md:inline; 
}

.error { 
  @apply mt-4; 
  @apply text-red-500; 
  @apply text-sm; 
  @apply flex; 
  @apply items-center; 
}

.result-wrap { 
  @apply flex; 
  @apply justify-center; 
  @apply mb-12; 
}

.result { 
  @apply bg-[#2d2d2d]; 
  @apply rounded-xl; 
  @apply p-6; 
  @apply transition-all; 
  @apply border; 
  @apply border-blue-500/30; 
  @apply flex; 
  @apply flex-col; 
  @apply relative; 
  @apply overflow-hidden; 
  @apply w-full; 
  @apply max-w-xl; 
  @apply min-h-[260px]; 
}

.result__badge { 
  @apply absolute; 
  @apply top-0; 
  @apply right-0; 
  @apply p-2; 
  @apply opacity-10; 
}

.result__badge-icon { 
  @apply text-6xl; 
}

.result__header { 
  @apply flex; 
  @apply justify-between; 
  @apply items-start; 
  @apply mb-4; 
  @apply relative; 
  @apply z-10; 
}

.result__header-left { 
  @apply flex-1; 
  @apply min-w-0; 
}

.result__title { 
  @apply text-xl; 
  @apply font-bold; 
}

.result__title--loading { 
  @apply text-blue-400; 
}

.result__title--ok { 
  @apply text-green-400; 
}

.result__ip { 
  @apply text-xs; 
  @apply text-gray-500; 
  @apply font-mono; 
  @apply mt-1; 
  @apply truncate; 
}

.result__header-right { 
  @apply bg-gray-800; 
  @apply text-xs; 
  @apply px-2; 
  @apply py-1; 
  @apply rounded; 
  @apply text-gray-400; 
  @apply whitespace-nowrap; 
  @apply ml-2; 
}

.result__content { 
  @apply space-y-2; 
  @apply mb-6; 
  @apply flex-grow; 
  @apply relative; 
  @apply z-10; 
}

.result__item { 
  @apply flex; 
  @apply justify-between; 
  @apply border-b; 
  @apply border-gray-700/50; 
  @apply pb-2; 
}

.result__item:last-child { 
  @apply border-b-0; 
  @apply pb-0; 
}

.result__item-value { 
  @apply font-medium; 
}

.result__item-label { 
  @apply text-gray-500; 
}

.skeleton { 
  @apply animate-pulse; 
  @apply space-y-3; 
}

.skeleton__line { 
  @apply h-4; 
  @apply bg-gray-700; 
  @apply rounded; 
}

.result__footer { 
  @apply flex; 
  @apply justify-between; 
  @apply items-center; 
  @apply pt-4; 
  @apply border-t; 
  @apply border-gray-700; 
  @apply mt-auto; 
  @apply text-sm; 
  @apply text-gray-500; 
  @apply relative; 
  @apply z-10; 
}

.result__status { 
  @apply flex; 
  @apply items-center; 
  @apply gap-2; 
}

.iface { 
  @apply bg-[#2d2d2d]; 
  @apply rounded-xl; 
  @apply p-8; 
  @apply border; 
  @apply border-gray-700; 
  @apply max-w-3xl; 
  @apply mx-auto; 
}

.iface__header { 
  @apply flex; 
  @apply items-center; 
  @apply justify-between; 
  @apply mb-4; 
}

.iface__title { 
  @apply text-2xl; 
  @apply font-bold; 
  @apply flex; 
  @apply items-center; 
  @apply gap-2; 
  @apply text-white; 
}

.iface__icon { 
  @apply text-blue-500; 
}

.iface__btn { 
  @apply px-4; 
  @apply py-2; 
  @apply rounded; 
  @apply bg-blue-600; 
  @apply hover:bg-blue-700; 
  @apply text-white; 
  @apply transition-colors; 
  @apply duration-200; 
}

.iface__desc { 
  @apply text-gray-400; 
}
</style>
