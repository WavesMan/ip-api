module.exports = {
  routes: [
    { path: "/api/ip", function: "./edge-functions/ip-lookup.js" },
    { path: "/api/stats", function: "./edge-functions/stats.js" },
  ],
};
