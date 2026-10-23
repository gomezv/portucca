# Publicación en npm

## Estado

El paquete `portucca` **está libre** en el registry (verificado el 2026-10-22) y la
publicación ocurre en la Fase 2 del roadmap, después de probar la política en `shucca` y
`seucca`.

## 1. Cuenta

- La cuenta se crea en **npmjs.com** (gratis): nombre de usuario, email verificado.
- **2FA es obligatorio** para publicar.
- Un paquete público es público para siempre: `npm unpublish` solo funciona dentro de las
  **72 horas** posteriores a la publicación y, aun así, el nombre queda bloqueado (solo
  quien lo publicó podría volver a usarlo). Por eso el nombre se decide antes de publicar
  (`docs/DECISIONS.md`, D1).
- Si se quiere un scope (`@ucca/portucca`), hace falta que el **usuario** `ucca` esté
  libre en npm; eso se verifica al registrarse (no es consultable de forma anónima).

## 2. Manifiesto previsto

```json
{
  "name": "portucca",
  "version": "0.1.0",
  "description": "Un puerto estable por worktree. Sticky, auditable, sin EADDRINUSE.",
  "type": "module",
  "bin": { "portucca": "bin/portucca.mjs" },
  "files": ["bin", "lib", "README.md", "LICENSE"],
  "engines": { "node": ">=20" },
  "scripts": {
    "test": "node --test test/"
  },
  "keywords": ["worktree", "ports", "dev", "monorepo", "agents"],
  "license": "MIT"
}
```

Reglas de `files`:

- Solo se publica lo que está en la lista. Ni tests, ni `.env`, ni registros de estado.
- El `README.md` se publica siempre (npm lo muestra en la página del paquete).
- `prepublishOnly` debería correr `npm test` y `npm pack --dry-run`.

## 3. Flujo

```bash
npm login                 # o: npm login --registry https://registry.npmjs.org
npm version 0.1.0         # tag git: v0.1.0
npm publish --access public
npm view portucca         # verificar
```

Verificación de instalación limpia (en un directorio temporal, sin nada local):

```bash
npx portucca@0.1.0 --help
npx portucca@0.1.0 doctor
```

## 4. Automatización (CI)

- Token de automatización de npm → secreto `NPM_TOKEN` en GitHub Actions.
- Workflow que publica cuando se empuja un tag `v*`:
  1. `npm ci`
  2. `npm test`
  3. `npm publish --access public` con `NODE_AUTH_TOKEN`
- El `package.json` de publicación necesita:

```json
"publishConfig": { "access": "public", "registry": "https://registry.npmjs.org/" }
```

## 5. Versionado y consumo

- Semver estricto: `0.1.0` mientras la política se estabiliza; breaking changes solo en
  `1.0.0`.
- **Los proyectos consumidores fijan la versión exacta** (`portucca@0.1.0`, sin `^`). Es el
  mismo criterio que ya se usa con Prisma `7.10`, AntD `5.29` o TypeScript `6`: en un
  worktree a las 2am, una actualización automática de la herramienta de puertos es una
  sorpresa innecesaria.
- La actualización del pin es una tarea explícita del proyecto, con su propio PR.

## 6. Lista de verificación antes de publicar

- [ ] `npm pack --dry-run` revisado archivo por archivo
- [ ] El paquete no incluye `.env`, registros de estado ni nada del proyecto consumidor
- [ ] `engines.node` refleja el mínimo real (Node 20+)
- [ ] El binario se ejecuta con `npx` sin instalar nada
- [ ] El README tiene: qué hace, instalación, ejemplo, política de fallos
- [ ] LICENSE en el paquete
- [ ] La versión publicada coincide con el tag git
