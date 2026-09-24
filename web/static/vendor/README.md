# Vendored third-party assets

Served locally so the console works fully offline (no CDN at runtime). Versions are pinned;
upgrade by replacing the file and updating this table. Verify integrity with:

```
sha256sum -c <(sed -n "s/^| \`\(.*\)\` | .* | \`\([0-9a-f]*\)\` |$/\2  \1/p" README.md)
```

| File | Package | SHA-256 |
|---|---|---|
| `dompurify/purify.min.js` | dompurify@3.4.16 | `2c90a9b46d6463f26038a29b686e82bc91de01fdac9d5229e7cfe3b360134ea2` |
| `marked/marked.min.js` | marked@11.1.1 | `f33a2d78362bde001670e3a29b01fd0dc16a7f9194d042a775af89270ab38b74` |
| `cytoscape/cytoscape.min.js` | cytoscape@3.27.0 | `dafc2088fb1e331cba988f488815373e0c29ebf2b202b11cda5527daf66debf1` |
| `elkjs/elk.bundled.js` | elkjs@0.9.2 | `4265f0268480ac9444c24a28837751ed926640ac0440c0300edbda91d2b9f18d` |
| `i18next/i18next.min.js` | i18next@23.11.5 | `9c17cab6ab49fda358227f1e1015fa8f2417614e37ce3d4756edf8269ad3c97c` |
| `xterm/xterm.js` | xterm@4.19.0 | `323bac53f364f9248f08c10c8daaeac91e848b5d413135b5b6c78d8dd255ef4c` |
| `xterm/xterm.css` | xterm@4.19.0 | `8ec01c834e8317e9214cc5b1ac47c2e6cb90ed74dc18c23e25904b2825af63f4` |
| `xterm-addon-fit/xterm-addon-fit.js` | xterm-addon-fit@0.5.0 | `f120d81c79b8069bbc362f9bf186f8724980a1c21afb3460f781e94e6db06bd1` |
| `fonts/inter-latin-wght-normal.woff2` | @fontsource-variable/inter@5 | `3100e775e8616cd2611beecfa23a4263d7037586789b43f035236a2e6fbd4c62` |
| `fonts/jetbrains-mono-latin-wght-normal.woff2` | @fontsource-variable/jetbrains-mono@5 | `18be452724bfdc236c074ca94a249a7f41a86752c7d04ab258ce9ed5651f6a7e` |

Icons: `../js/icons.js` is a generated subset of lucide-static@0.469.0 (ISC, see `LICENSE-lucide.txt`).

Security note: DOMPurify was upgraded from 3.0.8 to 3.4.16 to address GHSA-gx9m-whjm-85jf (CVE-2024-47875), GHSA-mmhx-hmjr-r674 (CVE-2024-45801) and later advisories.
