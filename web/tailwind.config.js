/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // Enterprise 中性色板（Linear / Vercel 系）
        bg:          "#FAFAFA",          // 画布
        bg2:         "#F4F4F5",          // segmented / chip 槽
        panel:       "#FFFFFF",          // 卡片
        panel2:      "#FAFAFA",          // 嵌套面板
        border:      "#E4E4E7",          // 1px 实线 hairline (zinc-200)
        borderHi:    "#D4D4D8",          // 强化 hairline (zinc-300)
        ink:         "#09090B",          // 主文字 (zinc-950)
        ink2:        "#27272A",          // 次主文字 (zinc-800)
        muted:       "#52525B",          // 次要 (zinc-600)
        muted2:      "#71717A",          // 三级 (zinc-500)
        placeholder: "#A1A1AA",          // 占位 (zinc-400)
        // 品牌强调 — 高对比近黑 CTA（Vercel 风）
        accent:      "#18181B",          // zinc-900
        accentDark:  "#000000",
        // 功能色 — 克制中性
        link:        "#2563EB",          // blue-600
        success:     "#16A34A",          // green-600
        warn:        "#CA8A04",          // amber-600
        danger:      "#DC2626",          // red-600
      },
      borderRadius: {
        ios: "8px",
        "ios-lg": "10px",
        "ios-xl": "12px",
      },
      boxShadow: {
        card:   "0 1px 0 rgba(0,0,0,0.04)",
        cardHi: "0 4px 12px rgba(0,0,0,0.06)",
        sticky: "0 -1px 0 #E4E4E7",
      },
      fontFamily: {
        sf: [
          "-apple-system", "BlinkMacSystemFont", "SF Pro Text", "SF Pro Display",
          "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Helvetica Neue",
          "Arial", "sans-serif",
        ],
      },
    },
  },
  plugins: [],
};
