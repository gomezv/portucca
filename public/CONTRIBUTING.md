# Contribuir

Este repositorio es un **espejo de releases** de `portucca`. El desarrollo ocurre en un
repositorio privado, así que los cambios no se aceptan como PRs directos aquí.

- Para reportar un bug o proponer un cambio, abre un issue.
- Cuando el cambio se implemente y se publique, aparecerá en este espejo.

## Reglas del proyecto

- Código en inglés; documentación en español.
- Node.js >= 20, ESM nativo, sin transpilador.
- Cero dependencias de runtime en v1.
- Linux primero: el bloqueo usa `flock`.
- Fail-fast: un conflicto de puertos falla con un mensaje accionable; nunca se reasigna en
  silencio.
- Commits convencionales, sin atribución de IA.

## Verificación

```bash
npm test
npm pack --dry-run
```
