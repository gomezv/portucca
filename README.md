# portucca

Un puerto estable por worktree. Sticky, auditable, sin `EADDRINUSE`.

Portucca reparte puertos de desarrollo entre los worktrees de un repositorio git (y entre
proyectos que comparten la misma máquina), de forma **pegajosa** (el mismo worktree siempre
obtiene los mismos puertos) y **fail-fast** (si hay conflicto, el proceso falla con un mensaje
explícito en vez de mudarse de puerto en silencio).

## Qué resuelve

| Problema | Qué hace portucca |
|---|---|
| Dos worktrees piden el mismo puerto | Asigna puertos por worktree con un registro global de claims y bloqueo (`flock`) |
| El puerto cambia en cada arranque | El claim sigue al **directorio** del worktree: si el stack baja y vuelve, la URL es la misma |
| Un worktree nuevo no arranca | Bootstrap declarativo: copia archivos desde el checkout principal y crea symlinks |
| Ya hay puertos en los archivos de entorno | `adoptFrom` registra el puerto existente sin hacer bind ni tumbar el stack |
| Dos proyectos en la misma máquina se pisan | Un único registro con claims etiquetados por proyecto |
| El checkout principal recibe escrituras | El main tiene puertos reservados y nunca entra al registro |

Portucca es agnóstico al lenguaje: funciona con N servicios por proyecto y no asume un
framework ni un layout de `.env` concreto.

## Qué NO resuelve (v1)

- Base de datos por worktree
- Supervisión de procesos, proxies por subdominio o certificados
- Contenedores / devcontainers
- Creación de worktrees: los crea `git worktree`, Orca u otra herramienta; portucca prepara el
  worktree existente
- Instalación de dependencias o builds: `setup` no ejecuta comandos del proyecto
- TUI y plugins (quedan para v2)

## Instalación

```bash
curl -fsSL https://raw.githubusercontent.com/gomezv/portucca/main/install.sh | bash
```

El script detecta Linux o macOS y la arquitectura (`amd64` o `arm64`), verifica el checksum e
instala en `${PORTUCCA_INSTALL_DIR:-$HOME/.local/bin}`. Para fijar una versión:

```bash
curl -fsSL https://raw.githubusercontent.com/gomezv/portucca/main/install.sh | PORTUCCA_VERSION=v0.1.0 bash
```

Con Go 1.23 o superior también se puede instalar desde el módulo:

```bash
go install github.com/gomezv/portucca/cmd/portucca@latest
```

Requisitos: Linux o macOS en `amd64` o `arm64`. Windows no tiene soporte de runtime en v1.

## Uso

```bash
portucca init --project app --service web:5173:5174 --service api:3004:3005
portucca setup    # prepara el worktree actual: bootstrap + ensure
portucca ensure   # asigna o recuerda los puertos; repara el entorno
portucca env      # muestra los valores resueltos sin escribir nada
```

`init` escribe `portucca.json` en la raíz del repo. El formato de `--service` es
`nombre:puerto_del_main:primer_puerto_de_worktrees`; con `--preset node-vite-nest` se rellenan
los targets de entorno del layout web+API.

Un `portucca.json` mínimo:

```json
{
  "version": 1,
  "project": "app",
  "services": {
    "web": { "mainPort": 5173, "startPort": 5174 },
    "api": { "mainPort": 3004, "startPort": 3005 }
  },
  "env": [
    { "service": "web", "file": "apps/web/.env.local", "key": "WEB_PORT" },
    { "service": "api", "file": "apps/api/.env.local", "key": "PORT" }
  ]
}
```

Después de crear un worktree, ejecuta `portucca setup` dentro de él. Los demás comandos son
`reallocate` (cambia los puertos del worktree de forma explícita), `prune` (limpia claims de
worktrees borrados), `doctor` (diagnóstico read-only), `list` y `version`.

## Integración

Todos los comandos aceptan `--json` y devuelven un sobre estable:

```json
{ "schemaVersion": 1, "command": "ensure", "ok": true, "data": {} }
```

Los exit codes son `0` (éxito), `1` (error accionable) y `2` (uso inválido). Una herramienta
que crea worktrees puede llamar `portucca setup --json` y leer los puertos sin parsear texto
humano.

## Estado

v1 en desarrollo. La instalación descrita aplica desde el primer release `v0.1.0`.

## Contribuir

Los issues y los PRs son bienvenidos. Abre un issue para bugs o propuestas y revisa
[`CONTRIBUTING.md`](CONTRIBUTING.md).

## Licencia

MIT. Ver [`LICENSE`](LICENSE).
