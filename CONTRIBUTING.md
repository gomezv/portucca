# Contribuir

Gracias por tu interés en portucca. Los issues y los PRs son bienvenidos.

## Proceso

1. Abre un issue para reportar un bug o proponer un cambio.
2. Haz un fork y una rama desde `main`.
3. Envía un PR con tests.
4. Los cambios aceptados se integran en la siguiente release.

## Reglas del proyecto

- Código e identificadores en inglés; documentación en español.
- Go 1.23 o superior, módulo único en la raíz del repositorio.
- stdlib primero; cero dependencias de terceros en v1 salvo justificación con evidencia.
- Linux y macOS en v1; Windows compila, pero falla con `unsupported_platform`.
- Fail-fast: un conflicto de puertos falla con un mensaje accionable; nunca se reasigna en
  silencio.
- Commits convencionales, sin atribución de IA.

## Verificación

```bash
go test -race ./...
go vet ./...
staticcheck ./...      # si está instalado
```

Antes de abrir el PR, comprueba también que `gofmt -l .` no devuelve archivos.
