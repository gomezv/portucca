# portucca

Un puerto estable por worktree. Sticky, auditable, sin `EADDRINUSE`.

Portucca reparte puertos de desarrollo entre los worktrees de un repositorio git (y entre
proyectos que comparten la misma máquina), de forma **pegajosa** (el mismo worktree siempre
obtiene los mismos puertos) y **fail-fast** (si hay conflicto, el proceso muere con un
mensaje explícito en vez de mudarse de puerto en silencio).

## Qué resuelve

| Problema | Qué hace portucca |
|---|---|
| Dos worktrees piden el mismo puerto | Asigna un bloque de puertos por worktree, con registro compartido y bloqueo (`flock`) |
| El puerto cambia en cada arranque | El claim sigue al **directorio** del worktree: si el stack baja y vuelve, la URL es la misma |
| Vite se muda de puerto solo | `strictPort: true`: conflicto = `EADDRINUSE` explícito, nunca drift silencioso |
| Un worktree sin `.env` no arranca | Bootstrap del entorno: copia el `.env` del checkout principal, symlinks y `.env.local` por worktree |
| Dos proyectos en la misma máquina se pisan | Un único registro a nivel máquina, con claim etiquetado por proyecto |

## Qué NO resuelve (v1)

- Base de datos por worktree
- Supervisión de procesos, proxies por subdominio o certificados
- Contenedores / devcontainers
- Orquestación de agentes: portucca no crea worktrees ni lanza nada; solo reparte puertos y
  escribe archivos de entorno

## Estado

Implementación **pendiente**. La política nació en un proyecto real y este repositorio la
extrae, la generaliza y la publica.

## Instalación (prevista)

```bash
npx portucca@0.1.0 ensure          # asigna (o recuerda) los puertos de este worktree
npx portucca@0.1.0 doctor          # diagnóstico: claims, conflictos, puertos ocupados
```

En los proyectos consumidores se instala como dependencia de desarrollo versionada:

```bash
pnpm add -D portucca@0.1.0
```

## Requisitos

- Linux (el bloqueo usa `flock` de util-linux)
- Node.js >= 20

## Contribuir

Los issues y los PRs son bienvenidos. Abre un issue para bugs o propuestas y revisa
[`CONTRIBUTING.md`](CONTRIBUTING.md).

## Licencia

MIT. Ver [`LICENSE`](LICENSE).
