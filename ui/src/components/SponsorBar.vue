<script setup>
defineProps({
  alipaySrc: { type: String, default: 'https://dq.waveyo.cn/assets/images/sponsorship/alipay.png' },
  wechatSrc: { type: String, default: 'https://dq.waveyo.cn/assets/images/sponsorship/wechat.png' },
})
// export default {
//   name: 'PaymentQRCode',
//   data() {
//     return {
//       alipaySrc: 'https://dq.waveyo.cn/assets/images/sponsorship/alipay.png',
//       wechatSrc: 'https://dq.waveyo.cn/assets/images/sponsorship/wechat.png'
//     }
//   }
// }
</script>

<template>
  <div class="sponsor">
    <div class="sponsor__header">
      <div class="sponsor__title">赞助支持</div>
      <div class="sponsor__desc">站点维护与 API 支持需要持续成本，期望获得一点微薄支持</div>
    </div>
    <div class="sponsor__grid">
      <div class="sponsor__item">
        <div class="sponsor__box">
          <img v-if="alipaySrc" :src="alipaySrc" alt="支付宝收款码" class="sponsor__img" />
          <div v-else class="sponsor__placeholder">支付宝二维码</div>
        </div>
        <div class="sponsor__label">支付宝</div>
      </div>
      <div class="sponsor__item">
        <div class="sponsor__box">
          <img v-if="wechatSrc" :src="wechatSrc" alt="微信收款码" class="sponsor__img" />
          <div v-else class="sponsor__placeholder">微信二维码</div>
        </div>
        <div class="sponsor__label">微信</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@reference "../style.css";
.sponsor { 
  @apply 
  bg-[#2d2d2d]; 
  @apply rounded-xl; 
  @apply p-6; 
  @apply border; 
  @apply border-gray-700; 
  @apply mb-6; 
}

.sponsor__header { 
  @apply flex; 
  @apply items-center; 
  @apply justify-between; 
  @apply mb-4; 
}

.sponsor__title { 
  @apply text-xl; 
  @apply font-bold; 
  @apply text-white; 
}

.sponsor__desc { 
  @apply text-sm; 
  @apply text-gray-400; 
}

.sponsor__grid { 
  @apply grid; 
  @apply grid-cols-1; 
  @apply sm:grid-cols-2; 
  @apply gap-6; 
}

.sponsor__item { 
  @apply flex; 
  @apply flex-col; 
  @apply items-center; 
  @apply gap-2; 
}

.sponsor__box { 
  @apply w-50; 
  @apply h-50; 
  @apply rounded-lg; 
  @apply overflow-hidden; 
  @apply border; 
  @apply border-gray-700;
  @apply bg-[#1a1a1a]; 
  @apply flex;
  @apply items-center; 
  @apply justify-center; 
}

.sponsor__img { 
  @apply w-full; 
  @apply h-full; 
  @apply object-cover; 
}

.sponsor__placeholder { 
  @apply text-gray-500; 
  @apply text-sm; 
}

.sponsor__label { 
  @apply text-gray-400; 
  @apply text-sm; 
}

</style>
