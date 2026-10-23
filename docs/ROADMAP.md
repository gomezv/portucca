# Roadmap

## Fase 0 · Adaptar la política a SEUCCA (antes de escribir el repo público)

El objetivo es que la política funcione en **dos** proyectos antes de publicarla.

- [ ] Crear el repo git de SEUCCA y el commit inicial de documentación (precondición:
      sin commit no hay worktrees)
- [ ] Portar `allocate-ports.mjs`, `lib/env-file.mjs` y `setup-worktree.sh` al esquema de
      SEUCCA (paquetes `@seucca/*`, bandas `5313`/`3314`)
- [ ] Adaptar la suite `node:test` y dejarla en verde
- [ ] Wrapper `.kilo/setup-script` y instrucción del hook de Orca (fuera del repo)
- [ ] `docs/DEV-PORTS.md` en SEUCCA con la política
- [ ] Ajustes al plan del scaffold de SEUCCA (ver `docs/ADOPTION-SEUCCA.md`)
- [ ] Prueba real: worktree temporal → setup → puertos reclamados → idempotencia → limpieza
- [ ] Commit de la infra en SEUCCA (sin push)

**Salida:** SEUCCA tiene la política funcionando y `shucca` no se toca.

## Fase 1 · Extraer al repo privado y espejo público

- [ ] `git init` en `portucca-internal` + estructura `public/` (código) y `docs/` (interno)
- [ ] Configurar `.github/workflows/sync-public.yml` (espejo `public/` → `gomezv/portucca`)
- [ ] Implementar la CLI en `public/` según `docs/SPEC.md` (comandos `ensure`,
      `reallocate`, `prune`, `doctor`, `setup`)
- [ ] Registro global con `project` etiquetado y bandas por proyecto
- [ ] Suite `node:test` completa, incluida la concurrencia real con `flock`
- [ ] `public/README.md` con la instalación y el ejemplo de uso
- [ ] Primer commit en el privado + primer sync al público, declarando el origen
      (port de `shucca` PR #22) con atribución

**Salida:** la CLI funciona en local y el espejo público se actualiza desde el privado.

## Fase 2 · Publicar `0.1.0` en npm

- [ ] Cuenta npm creada, 2FA activo, token de automatización configurado
- [ ] `package.json` con `bin`, `files`, `engines`
- [ ] `npm pack --dry-run` revisado (qué se publica y qué no)
- [ ] Publicación + verificación de instalación limpia (`npx portucca@0.1.0 --help`)
- [ ] Workflow de publicación por tag (`NPM_TOKEN` en GitHub Actions)
- [ ] Anuncio: README del repo, tag, y mención en los proyectos que lo consumen

**Salida:** `npx portucca@0.1.0` funciona en cualquier máquina.

## Fase 3 · Migrar los consumidores

- [ ] SEUCCA consume `portucca` como dependencia de desarrollo versionada
- [ ] `shucca` migra su registro al archivo global (PR pequeño)
- [ ] `shucca` consume `portucca` y retira su copia local de los scripts
- [ ] Los guards de API y web de `shucca` quedan idénticos a los del portucca

**Salida:** una sola implementación, un solo registro, dos proyectos.

## Fase 4 · Decisiones diferidas

- [ ] Base de datos por worktree (D7): activar por proyecto cuando el flujo lo pida
- [ ] Contenedores por worktree: evaluar si el aislamiento total sustituye al allocator
- [ ] Soporte de macOS/Windows, si hay demanda
- [ ] Nombres de proyecto y bandas por defecto para proyectos de terceros

## Preguntas abiertas

| Pregunta | Estado | Nota |
|---|---|---|
| ¿El registro global es un archivo o un directorio por proyecto? | Propuesta: un archivo con `projects` y `worktrees` | Un archivo es más simple de bloquear con `flock`; un directorio escala mejor |
| ¿Los puertos por defecto de un proyecto nuevo se derivan del nombre? | Pendiente | Hoy se configuran por proyecto; la derivación automática es decisión de v2 |
| ¿El nombre del paquete cambia si el scope crece? | D1 deja `ucca` como nombre de reserva | Renombrar en npm tras publicar es costoso: mejor decidir antes de `0.1.0` |
| ¿Se publica también una imagen de Docker? | Pendiente | Sin valor mientras elAllocator no gestione contenedores |
| ¿Se integra con el ledger de trabajo de los agentes (Orca)? | Fuera de v1 | Portucca no crea worktrees ni conoce agentes |

## Criterios de "listo para publicar"

- [ ] La CLI cubre los comandos de `docs/SPEC.md` con el comportamiento especificado
- [ ] La suite de tests pasa en local y en CI
- [ ] `shucca` y `seucca` pueden migrar sin cambiar el comportamiento observable de sus
      stacks (mismos puertos, mismos mensajes de error)
- [ ] El README explica la instalación, el uso y la política sin necesidad de leer el código
- [ ] El espejo público se actualiza desde el privado y no contiene docs internos
- [ ] `npm pack --dry-run` no incluye nada que no deba publicarse
