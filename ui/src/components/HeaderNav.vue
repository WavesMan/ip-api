<script setup>
</script>

<template>
  <header class="header">
    <div class="header__container">
      <div class="header__brand">
        <i class="fas fa-globe-asia"></i>
        <span>IP 归属地查询</span>
      </div>
      <nav class="nav">
        <ul class="nav__list">
          <li>
            <RouterLink to="/home" class="nav__link" active-class="link-active">首页</RouterLink>
          </li>
          <li>
            <RouterLink to="/docs" class="nav__link" active-class="link-active">接口说明</RouterLink>
          </li>
          <li><a href="https://home.waveyo.cn" target="_blank" class="nav__link">WaveYo Home</a></li>
          <li><a href="https://blog.waveyo.cn" target="_blank" class="nav__link">WaveYo Blog</a></li>
          <li><a href="https://github.com/WavesMan" target="_blank" class="nav__link">GitHub</a></li>
        </ul>
      </nav>
    </div>
  </header>
</template>

<style scoped>
@reference "../style.css";
.header {
  @apply border-b;
  @apply border-gray-700;
  @apply bg-[#1a1a1a];
  @apply sticky;
  @apply top-0;
  @apply z-50;
}

.header__container {
  @apply container;
  @apply mx-auto;
  @apply px-4;
  @apply py-4;
  @apply flex;
  @apply flex-wrap;
  @apply items-center;
  @apply justify-between;
  @apply gap-3;
}

.header__brand {
  @apply text-2xl;
  @apply font-bold;
  @apply text-blue-500;
  @apply flex;
  @apply items-center;
  @apply gap-2;
}

.nav {
  @apply w-full;
  @apply sm:w-auto;
}

.nav__list {
  @apply flex;
  @apply flex-wrap;
  @apply gap-3;
  @apply sm:gap-6;
  @apply text-gray-300;
  @apply justify-start;
  @apply sm:justify-end;
}

.nav__link {
  @apply hover:text-blue-500;
  @apply transition-colors;
}

.link-active { 
  @apply text-blue-500; 
}
</style>
