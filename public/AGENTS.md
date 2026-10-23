# AGENTS.md — portucca

Este repositorio es un **espejo de solo lectura**. No commitees cambios aquí: se sincronizan
desde el repositorio privado de desarrollo.

## Reglas del código

- Código en inglés; documentación en español.
- Node.js >= 20, ESM nativo (`"type": "module"`), sin transpilador.
- Cero dependencias de runtime en v1.
- Linux primero: si falta `flock`, fallar con un mensaje claro.
- Fail-fast: nunca reasignar un puerto en silencio.
- Tests: `npm test` (`node --test`).
- Commits convencionales, sin atribución de IA.
