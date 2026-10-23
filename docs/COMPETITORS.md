# Análisis competitivo

Medición tomada el **2026-10-22** (versiones publicadas y descargas mensuales reales del
registry de npm, no de los README). El objetivo no es elegir un ganador ajeno, sino
justificar qué construye portucca y cuándo vale la pena cambiar de estrategia.

## 1. Herramientas Purpose-built

| Herramienta | Enfoque | Estado (2026-10-22) | Descargas/mes | Qué resuelve | Qué no resuelve |
|---|---|---|---|---|---|
| [portweave](https://github.com/gameweave/portweave) | Pool **global de máquina**, sticky por worktree, detección de conflictos, inyección de env | npm `0.8.1`, MIT | 2 673 | Dos proyectos que piden `5173` no chocan; sticky; sin daemon | Inyecta variables de entorno, lo que choca con **Turbo Strict Mode** (que filtra las env no declaradas) |
| [portlock](https://github.com/johndockery/portlock) | Rango base determinístico por worktree, claim store global | npm `0.1.1`, MIT | 3 347 | Identidad estable por worktree; derivación de URLs | No gestiona procesos, ni bases de datos, ni contenedores; claims con vida corta |
| [worktree-env](https://github.com/cedricvidal/worktree-env) | Offset por worktree sobre `.env.base` (base+1, base+2…) | npm `0.1.2`, MIT | 1 337 | Modelo simple y legible; aísla `COMPOSE_PROJECT_NAME` | Requiere puertos base con ceros finales; sin detección de colisiones externas |
| [branch2ports](https://github.com/chaspy/branch2ports) | Hash de repo + rama → puertos | npm `0.1.4`, MIT | 15 | Cero estado, cero configuración | Sin detección de colisiones; hashes pueden pisarse |
| [portool](https://github.com/t09tanaka/portool) | Ledger global con `flock`, bloque por worktree, hook `post-checkout` | binario (fuera de npm) | — | Ledger cooperativo, el más parecido al modelo de portucca | **Por defecto es pasivo**: avisa en vez de fallar; contradice la filosofía fail-fast |
| [portree](https://github.com/fairy-pitta/portree) | Gestor de servidores por worktree (Go, TUI) | proyecto nuevo | — | Ciclo de vida de procesos, proxy por subdominio, HTTPS | invade mucha superficie: quedaría por debajo de la línea del problema |
| [devx](https://github.com/jfox85/devx) | Sesiones de dev con worktree, puertos, Caddy, tmux | proyecto nuevo | — | Orquestación completa de sesión | Producto grande para un problema de puertos |
| [wtdev](https://github.com/Dave-56/wtdev) | Puerto como **función pura** del path del worktree, sin estado | proyecto nuevo (fuera de npm) | — | Cero coordinación, cero registro | Sin detección de occupation externa; hash puede colisionar |
| [vibetree](https://github.com/lexun/vibetree) | Variables por worktree (puertos, IDs, instancias) | early development | — | Plantillas y auto-incrementos | APIs en movimiento; Early Development según su propio README |
| [worktrunk](https://worktrunk.dev) | Producto: hooks, `hash_port`, DB por worktree, Caddy, `direnv`/`mise` | producto comercial | — | Flujo completo de worktrees con Hooks | Producto cerrado; nuestro caso es más acotado |

## 2. Las primitivas realmente conocidas

| Herramienta | Qué resuelve | Por qué no es la respuesta aquí |
|---|---|---|
| **direnv** / **mise** | Variables de entorno por directorio, automáticas al hacer `cd` | Resuelven el *env* por worktree, no la *asignación de puertos*. Además Vite y NestJS ya leen `.env.local` de forma nativa |
| **devcontainer** / **devbox** | Namespace de red por worktree: todos pueden usar el mismo puerto | Aíslan del todo, pero exigen contenedor (y con contenedor, el puerto deja de ser el problema) |
| **docker compose** (`-p`, `COMPOSE_PROJECT_NAME`) | Aísla recursos Docker por proyecto | Aísla contenedores, no el proceso de desarrollo en el host |
| **Tilt** | Desarrollo multi-servicio con live update y port forwarding | Orientado a stacks de contenedores, no a un monorepo TypeScript |

## 3. Qué hace portucca y por qué no es "otro asignador de puertos"

Tres diferencias respecto a lo que existe:

1. **Fail-fast es el contrato, no una opción.** Casi todas las herramientas anteriores
   avisan y siguen (`portool` por defecto ni siquiera comprueba el puerto; `wtdev` y
   `branch2ports` no comprueban nada). En un flujo con agentes ejecutando scripts en
   paralelo, un warning que se ignora es exactamente el modo de fallo que duele: dos
   agentes creyendo que están en su propio puerto.
2. **`.env.local` como fuente autoritativa, no variables inyectadas.** El stack de shucca
   corre sobre Turbo Strict Mode, que existe precisamente para que el entorno sea
   explícito y no dependa de lo que herede el shell. Un allocator que inyecta `PORT` en el
   proceso hijo tiene que declararlo en `turbo.json` o se pierde en el camino.
3. **Bootstrap de entorno incluido.** Portucca no solo reparte puertos: copia el `.env` del
   checkout principal, crea el symlink `apps/api/.env -> ../../.env` y deja el worktree en
   condición de arrancar. Es la mitad del trabajo que hoy vive en hooks invisibles.

## 4. Cuándo reconsiderar

| Señal | Acción |
|---|---|
| Un tercero de la familia (más allá de shucca y seucca) con necesidades distintas | Evaluar `portucca` vs `portweave` con datos de uso reales |
| `portweave` llega a 1.0 y resuelve la inyección de env compatible con Turbo | Migrar si el mantenimiento propio supera el beneficio |
| El scope crece a "gestor de entornos de desarrollo" | Renombrar a `ucca` (el nombre de la familia) antes de publicar breaking changes |
| Aparecen 3+ proyectos con policies distintas | Extraer el núcleo a un paquete y dejar portucca como la CLI de la familia |

**Hoy no se adopta ninguna herramienta externa**: el flujo crítico está resuelto, probado y
fusionado en `shucca`; migrar ahora costaría más de lo que ahorra. El repo existe para
compartir la política con proyectos que no son suyos y para que la política deje de estar
escondida en un hook de agente.
