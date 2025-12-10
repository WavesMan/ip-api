<script setup>
import SponsorBar from '../components/SponsorBar.vue'
import { ref, onMounted } from 'vue'

const dataSourceName = ref(typeof window !== 'undefined' ? (window.__DATA_SOURCE__ || 'IPIP 数据库') : 'IPIP 数据库')
const dataSourceUrl = ref(typeof window !== 'undefined' ? (window.__DATA_SOURCE_URL__ || 'https://www.ipip.net') : 'https://www.ipip.net')
const apiBase = ref(typeof window !== 'undefined' ? (window.__API_BASE__ || '/api') : '/api')

onMounted(() => {})
</script>

<template>
  <div class="docs">
    <SponsorBar />

    <div class="datasource">
      <div class="datasource__title">
        <i class="fas fa-database datasource__icon"></i>
        <span>聚合 数据源声明</span>
      </div>
      <div class="datasource__list">
        <div class="datasource__item">
          <span class="datasource__name">{{ dataSourceName }}</span>
          <a :href="dataSourceUrl" target="_blank" class="datasource__link">{{ dataSourceUrl }}</a>
        </div>
        <div class="datasource__item">
          <span class="datasource__name">高德 API</span>
          <a href="https://lbs.amap.com/" target="_blank" class="datasource__link">https://lbs.amap.com/</a>
        </div>
        <div class="datasource__item">
          <span class="datasource__name">自有数据源</span>
        </div>
      </div>
    </div>

    <h2 class="docs__title"><i class="fas fa-book docs__icon"></i> 使用说明</h2>
    <div class="docs__blocks">
      <div>
        <h3 class="docs__block-title docs__block-title--blue">接口（后端查询）</h3>
        <div class="docs__code">
          <div>GET https://ip.waveyo.cn{{ apiBase }}/ip?ip=8.8.8.8</div>
          <div>GET https://ip.waveyo.cn{{ apiBase }}/stats</div>
        </div>
      </div>
      <div>
        <h3 class="docs__block-title docs__block-title--green">示例响应</h3>
        <div class="docs__code"><pre>{
  "ip": "8.8.8.8",
  "country": "美国",
  "region": "北美",
  "province": "加利福尼亚",
  "city": "山景城",
  "isp": "Google LLC"
}</pre></div>
      </div>
      <div>
        <h3 class="docs__block-title docs__block-title--green">统计响应示例</h3>
        <div class="docs__code"><pre>{
  "total": 12345,
  "today": 67
}</pre></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@reference "../style.css";
.docs { 
  @apply container; 
  @apply mx-auto; 
  @apply px-4; 
  @apply py-8; 
}

.docs__title { 
  @apply text-2xl; 
  @apply font-bold; 
  @apply mb-6; 
  @apply flex; 
  @apply items-center; 
  @apply gap-2; 
  @apply text-white; 
}

.docs__icon { 
  @apply text-blue-500; 
}

.docs__blocks { 
  @apply space-y-8; 
}

.datasource { 
  @apply mb-6; 
  @apply bg-[#1a1a1a]; 
  @apply p-4; 
  @apply rounded-lg; 
  @apply border; 
  @apply border-gray-700; 
  @apply text-sm; 
  @apply text-gray-300; 
}

.datasource__title { 
  @apply flex; 
  @apply items-center; 
  @apply gap-2; 
  @apply mb-2; 
}

.datasource__list { 
  @apply space-y-1; 
}

.datasource__item { 
  @apply flex; 
  @apply items-center; 
  @apply justify-between; 
}

.datasource__icon { 
  @apply text-blue-500; 
}

.datasource__name { 
  @apply font-semibold; 
  @apply text-blue-400; 
}

.datasource__link { 
  @apply text-blue-500; 
  @apply hover:text-blue-400; 
  @apply underline; 
}

.docs__block-title { 
  @apply text-lg; 
  @apply font-semibold; 
  @apply mb-2; 
}

.docs__block-title--blue { 
  @apply text-blue-400; 
}

.docs__block-title--purple { 
  @apply text-purple-400; 
}

.docs__block-title--green { 
  @apply text-green-400; 
}

.docs__code { 
  @apply bg-[#1a1a1a]; 
  @apply p-4; 
  @apply rounded-lg; 
  @apply font-mono; 
  @apply text-sm; 
  @apply border; 
  @apply border-gray-700; 
  @apply text-gray-300; 
}
</style>
