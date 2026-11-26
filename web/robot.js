// J.A.R.V.I.S. arc reactor — SVG animado (o nome JarvisRobot ficou por compatibilidade).
// Estados: idle | listening | thinking | speaking | sleep
(function () {
  const css = `
  .jrobot{display:block;overflow:visible;--lvl:0}
  .jrobot .spin1{transform-origin:100px 100px;animation:jr-spin 16s linear infinite}
  .jrobot .spin2{transform-origin:100px 100px;animation:jr-spin 10s linear infinite reverse}
  .jrobot .spin3{transform-origin:100px 100px;animation:jr-spin 24s linear infinite}
  @keyframes jr-spin{to{transform:rotate(360deg)}}
  .jrobot .core{transform-origin:100px 100px;transform:scale(calc(1 + var(--lvl) * .38));transition:transform .08s}
  .jrobot .coreglow{transform-origin:100px 100px;transform:scale(calc(1 + var(--lvl) * .7));opacity:calc(.55 + var(--lvl) * .45)}
  .jrobot .breathe{transform-origin:100px 100px;animation:jr-breathe 3s ease-in-out infinite}
  @keyframes jr-breathe{50%{transform:scale(.94);opacity:.75}}
  .jrobot .pulse{transform-origin:100px 100px;opacity:0}
  .jrobot .coil{transition:stroke .4s}
  /* ouvindo: anel de pulso */
  .jrobot.listening .pulse{animation:jr-pulse 1.6s ease-out infinite}
  @keyframes jr-pulse{from{transform:scale(.85);opacity:.9}to{transform:scale(1.25);opacity:0}}
  /* pensando: tudo gira rápido e fica dourado */
  .jrobot.thinking .spin1{animation-duration:2.2s}
  .jrobot.thinking .spin2{animation-duration:1.4s}
  .jrobot.thinking .spin3{animation-duration:3s}
  .jrobot.thinking .coil{stroke:#ffcf5c!important}
  .jrobot.thinking .breathe{animation-duration:.8s}
  /* dormindo (mic desligado) */
  .jrobot.sleep .core,.jrobot.sleep .coreglow{opacity:.25}
  .jrobot.sleep .spin1,.jrobot.sleep .spin2,.jrobot.sleep .spin3{animation-play-state:paused}
  /* F.R.I.D.A.Y.: núcleo em forma de coração */
  .jrobot .heart{display:var(--fri,none)}
  .jrobot .round{display:var(--ball,inline)}
  `;
  if (!document.getElementById("jrobot-css")) {
    const st = document.createElement("style"); st.id = "jrobot-css"; st.textContent = css; document.head.appendChild(st);
  }
  let n = 0;
  function svg(size, extraClass) {
    const id = "jr" + (++n);
    // 10 bobinas do reator
    let coils = "";
    const p = (r, ang) => `${(100 + r * Math.cos(ang)).toFixed(2)} ${(100 + r * Math.sin(ang)).toFixed(2)}`;
    for (let i = 0; i < 10; i++) {
      const a = (i / 10) * Math.PI * 2, a2 = a + (Math.PI * 2 / 10) * 0.62;
      coils += `<path class="coil" d="M${p(46, a)} A46 46 0 0 1 ${p(46, a2)} L${p(60, a2)} A60 60 0 0 0 ${p(60, a)} Z" style="fill:rgba(var(--c-rgb),.16);stroke:var(--cyan)" stroke-width="1.4"/>`;
    }
    return `<svg class="jrobot idle ${extraClass || ""}" width="${size}" height="${size}" viewBox="0 0 200 200" aria-label="J.A.R.V.I.S.">
  <defs>
    <radialGradient id="${id}g" cx=".5" cy=".5" r=".5"><stop offset="0" style="stop-color:var(--cyan)" stop-opacity=".55"/><stop offset="1" style="stop-color:var(--cyan)" stop-opacity="0"/></radialGradient>
    <radialGradient id="${id}c" cx=".5" cy=".45" r=".55"><stop offset="0" stop-color="#ffffff"/><stop offset=".45" style="stop-color:var(--core)"/><stop offset="1" style="stop-color:var(--cyan)"/></radialGradient>
    <filter id="${id}f" x="-60%" y="-60%" width="220%" height="220%"><feGaussianBlur stdDeviation="3" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
  </defs>
  <circle class="coreglow" cx="100" cy="100" r="96" fill="url(#${id}g)"/>
  <circle class="pulse" cx="100" cy="100" r="88" fill="none" style="stroke:var(--cyan)" stroke-width="2"/>
  <circle cx="100" cy="100" r="92" fill="none" style="stroke:var(--cyan)" stroke-opacity=".25" stroke-width="1"/>
  <g class="spin1"><circle cx="100" cy="100" r="84" fill="none" style="stroke:var(--cyan)" stroke-width="3" stroke-dasharray="34 10 6 10" filter="url(#${id}f)"/></g>
  <g class="spin2"><circle cx="100" cy="100" r="72" fill="none" style="stroke:var(--cyan2)" stroke-width="7" stroke-opacity=".7" stroke-dasharray="2 7"/></g>
  <g class="spin3" filter="url(#${id}f)">${coils}</g>
  <circle cx="100" cy="100" r="40" fill="none" style="stroke:var(--cyan)" stroke-width="2" stroke-opacity=".8"/>
  <g class="breathe">
    <circle class="coreglow" cx="100" cy="100" r="34" fill="url(#${id}g)"/>
    <g class="core" filter="url(#${id}f)">
      <circle class="round" cx="100" cy="100" r="20" fill="url(#${id}c)"/>
      <path class="heart" d="M100 122 C70 101 76 78 100 90 C124 78 130 101 100 122 Z" fill="url(#${id}c)"/>
    </g>
  </g>
</svg>`;
  }
  function set(el, state) {
    if (!el) return;
    el.classList.remove("idle", "listening", "thinking", "speaking", "sleep");
    el.classList.add(state);
    if (state !== "speaking" && state !== "listening") level(el, 0);
  }
  function level(el, v) {
    if (!el) return;
    v = Math.max(0, Math.min(1, v || 0));
    el.style.setProperty("--lvl", v.toFixed(3));
  }
  window.JarvisRobot = { svg, set, level };
})();
