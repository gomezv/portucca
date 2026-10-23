# Especificación de portucca

Estado: **borrador de contrato** (la implementación aún no existe; el comportamiento
descrito aquí es el que se extrajo de `shucca` PR #22 y se generalizó).

## 1. Principios

| # | Principio | Consecuencia observable |
|---|---|---|
| P1 | **Sticky** | El mismo worktree obtiene los mismos puertos en cada ejecución, aunque el stack esté apagado |
| P2 | **Fail-fast** | Un conflicto produce error con instrucción; nunca una mudanza silenciosa de puerto |
| P3 | **Global a la máquina** | Un solo registro con claims de todos los proyectos: dos proyectos nunca se pisan |
| P4 | **El claim sigue al directorio** | El puerto se libera al borrar el worktree, no al detener el proceso |
| P5 | **El main está reservado** | El checkout principal nunca recibe claim ni escritura automática |
| P6 | **Sin red, sin daemon** | Todo es local: archivos, `flock` y sockets TCP |

## 2. Alcance de la v1

**Dentro:**

- Registro global de claims (`$XDG_STATE_HOME/portucca/ports.json`), etiquetado por proyecto
- Asignación de puertos web/API por worktree, con bandas por proyecto configurables
- Bootstrap de entorno por worktree: `.env` desde el checkout principal, symlink
  `apps/api/.env -> ../../.env`, `.env.local` con el puerto de cada proceso
- Guards de arranque: API y web fallan si un worktree intenta usar el puerto del main
- CLI: `ensure`, `reallocate`, `prune`, `doctor`, `setup`, salida `--json`

**Fuera (v1):**

| Fuera | Por qué |
|---|---|
| Base de datos por worktree | Decisión diferida (`docs/DECISIONS.md`, D7) |
| Supervisión de procesos / TUI | ElAllocator reparte; no arranca ni mata nada |
| Proxy por subdominio, HTTPS, certificados | Resuelve un problema distinto (el acceso remoto, no el conflicto) |
| Contenedores / devcontainers | Otra estrategia de aislamiento, no complemento |
| Creación de worktrees | `git worktree` y las herramientas de agentes ya lo hacen |
| Multi-tenant / lógica de negocio | Fuera del dominio de una herramienta de puertos |

## 3. Contrato de la CLI

```
portucca ensure      # asigna o recuerda los puertos del worktree actual (idempotente)
portucca reallocate  # libera el claim propio y reasigna (único modo que cambia la URL)
portucca prune       # elimina claims cuyo worktree ya no existe
portucca doctor      # diagnóstico: claims, puertos ocupados, conflictos, estado del entorno
portucca setup       # bootstrap completo: env + puertos + (opcional) install/build
```

Opciones globales:

| Flag | Efecto |
|---|---|
| `--json` | Salida en JSON por stdout; los mensajes humanos van a stderr |
| `--project <name>` | Sobrescribe el nombre de proyecto (por defecto se deriva de la raíz del repo) |
| `--registry <path>` | Sobrescribe la ruta del registro (por defecto `$XDG_STATE_HOME/portucca/ports.json`) |

Códigos de salida: `0` éxito · `1` error accionable (conflicto, falta `flock`, claim de otro
worktree) · `2` uso inválido.

### 3.1 `ensure`

Es el comando idempotente del día a día. No reasigna si ya existe claim propio.

| Situación | Acción |
|---|---|
| Claim propio en el registro | No reasigna. Solo repara `.env.local` si falta o no coincide |
| Sin claim, `.env.local` con puertos adoptables | **Adopta** esos puertos (sin test de bind) y los registra como `adopted: true` |
| Sin claim y sin puertos adoptables | Asigna el primer puerto libre de la banda del proyecto |
| Puerto reclamado por **otro** worktree (vivo) | No lo toca. Error con instrucción de `--reallocate` |

Salida JSON:

```json
{
  "project": "seucca",
  "worktreePath": "/abs/path/al/worktree",
  "isMain": false,
  "webPort": 5314,
  "apiPort": 3315,
  "adopted": true,
  "changed": true,
  "envFiles": {
    "web": "/abs/path/apps/web/.env.local",
    "api": "/abs/path/apps/api/.env.local"
  }
}
```

En el checkout principal: `isMain: true`, `webPort`/`apiPort` = puertos reservados del
proyecto, `adopted: false`, `changed: false` y **cero escrituras**.

### 3.2 `reallocate`

Libera el claim propio y asigna el siguiente bloque libre. Es la **única** operación que
cambia la URL de un worktree, y por eso es explícita. En el checkout principal es un error.

### 3.3 `prune`

Elimina los claims cuyo directorio de worktree ya no existe (comparación por `realpath`).
Idempotente.

### 3.4 `doctor`

No modifica nada. Reporta: claims registrados, puertos ocupados por procesos ajenos, claims
apuntando a directorios inexistentes, y si el entorno del worktree actual está completo
(`.env`, symlink, `.env.local`).

## 4. Registro de claims

Ruta por defecto: `$XDG_STATE_HOME/portucca/ports.json` (fallback
`~/.local/state/portucca/ports.json`). Permisos `0600`, directorio `0700`.

```json
{
  "version": 1,
  "projects": {
    "seucca": { "mainWebPort": 5313, "mainApiPort": 3314, "webPortStart": 5314, "apiPortStart": 3315 },
    "shucca": { "mainWebPort": 5173, "mainApiPort": 3004, "webPortStart": 5174, "apiPortStart": 3005 }
  },
  "worktrees": [
    {
      "project": "seucca",
      "worktreePath": "/abs/path/al/worktree",
      "webPort": 5314,
      "apiPort": 3315,
      "adopted": true,
      "updatedAt": "2026-10-22T00:00:00.000Z"
    }
  ]
}
```

Es escritura atómica (archivo temporal + `rename`) y siempre bajo `flock` exclusivo sobre
`<registro>.lock`.

## 5. Bandas de puertos

Cada proyecto declara su propia banda para que dos proyectos en la misma máquina no
compartan familia. Ver `docs/ADOPTION-SEUCCA.md` para los valores de SEUCCA y
`docs/DESIGN.md` para el criterio.

Reglas:

- El checkout principal usa los puertos reservados del proyecto y **no** entra en el
  registro.
- Los worktrees empiezan en `webPortStart` / `apiPortStart` y suben de uno en uno.
- Un puerto se considera libre solo si se puede hacer bind en `0.0.0.0` **y** en `::` (los
  dos listen de Vite y Node usan familias distintas).
- Un puerto reclamado por otro worktree **vivo** no se reutiliza, aunque esté libre.

## 6. Bootstrap de entorno (`setup`)

1. Localiza el checkout principal con `git rev-parse --git-common-dir` (no depende de
   variables de entorno del agente).
2. Si falta `.env` en el worktree, lo copia del principal. **Nunca** pisa uno existente.
3. Si falta `apps/api/.env`, crea el symlink `apps/api/.env -> ../../.env`. Si existe como
   archivo regular, lo migra a symlink conservando su `PORT` en `.env.local`.
4. Ejecuta `ensure`.
5. Opcional: instala dependencias y compila los packages compartidos, si el workspace existe.

## 7. Guards de arranque

**API** (solo en desarrollo):

| Condición | Comportamiento |
|---|---|
| Faltan `JWT_PRIVATE_KEY` / `JWT_PUBLIC_KEY` | Falla con mensaje que nombra `portucca setup` |
| El checkout es un worktree y el puerto resuelto es el del main | Falla: el worktree no tiene `.env.local` |
| Falla de conexión a la base de datos | Traduce `PrismaClientInitializationError` a un mensaje accionable |

**Web**: si el repo es un worktree y el puerto configurado es el del main, falla nombrando
`portucca setup`. En `build` el guard no corre (producción intacta).

## 8. CORS en desarrollo

En dev se acepta cualquier origen de loopback con puerto explícito
(`http://localhost:<puerto>`, `127.0.0.1`, `[::1]`) porque el puerto del worktree es
variable. En producción, allowlist explícita desde `CORS_ORIGINS`. Nunca
`origin: true` junto con `credentials: true`.
