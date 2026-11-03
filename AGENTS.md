# AGENTS.md — portucca

Reglas para trabajar en este repositorio.

## Reglas del código

- Código e identificadores en inglés; documentación en español.
- Go 1.27 o superior. El módulo vive en la raíz del repositorio (`go.mod`) y produce un
  binario único.
- stdlib primero; cero dependencias de terceros en v1 salvo justificación con evidencia.
- `gofmt`, `go vet` y `staticcheck` limpios antes de commitear.
- Tests: `go test -race ./...`.
- Linux y macOS en v1. Windows compila con un stub que falla con `unsupported_platform`.
- Fail-fast: un conflicto de puertos falla con un mensaje accionable; nunca se reasigna en
  silencio.
- Commits convencionales, sin atribución de IA.

## Alcance

- Portucca no crea worktrees, no instala dependencias, no ejecuta builds y no supervisa
  procesos.
- El registro de claims es global a la máquina; las bandas viven en `portucca.json`, dentro
  del repo.
- El checkout principal nunca recibe claims ni escrituras automáticas.
