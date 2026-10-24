# Contribuir

Gracias por tu interés en portucca. Los issues y los PRs son bienvenidos.

## Proceso

1. Abre un issue para reportar un bug o proponer un cambio.
2. Haz un fork y una rama desde `main`.
3. Envía un PR con tests.
4. Los cambios aceptados se integran en la siguiente release.

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
