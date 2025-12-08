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
  <div class="bg-[#2d2d2d] rounded-xl p-6 border border-gray-700 mb-6">
    <div class="flex items-center justify-between mb-4">
      <div class="text-xl font-bold text-white">赞助支持</div>
      <div class="text-sm text-gray-400">站点维护与 API 支持需要持续成本，期望获得一点微薄支持</div>
    </div>
    <div class="grid grid-cols-2 gap-6">
      <div class="flex flex-col items-center gap-2">
        <div class="w-50 h-50 rounded-lg overflow-hidden border border-gray-700 bg-[#1a1a1a] flex items-center justify-center">
          <img 
            v-if="alipaySrc" 
            :src="alipaySrc" 
            alt="支付宝收款码" 
            class="w-full h-full object-cover" 
          />
          <div v-else class="text-gray-500 text-sm">支付宝二维码</div>
        </div>
        <div class="text-gray-400 text-sm">支付宝</div>
      </div>
      <div class="flex flex-col items-center gap-2">
        <div class="w-50 h-50 rounded-lg overflow-hidden border border-gray-700 bg-[#1a1a1a] flex items-center justify-center">
          <img 
            v-if="wechatSrc" 
            :src="wechatSrc" 
            alt="微信收款码" 
            class="w-full h-full object-cover" 
          />
          <div v-else class="text-gray-500 text-sm">微信二维码</div>
        </div>
        <div class="text-gray-400 text-sm">微信</div>
      </div>
    </div>
  </div>
</template>
