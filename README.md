# portucca-internal

Repositorio privado de trabajo de **portucca**.

- `public/`: lo único que se publica, como espejo, en <https://github.com/gomezv/portucca>.
- `docs/`: especificación, diseño, decisiones, roadmap, competencia y notas de adopción. No se publica.
- `.github/workflows/sync-public.yml`: sincroniza `public/` con el repo público al mergear en `main`.

## Cómo se trabaja

1. Rama + PR contra `main` de este repo.
2. El código y la documentación pública se editan en `public/`.
3. Las decisiones y el diseño se editan en `docs/`.
4. Al mergear, el workflow hace `git subtree split --prefix=public` y empuja a `gomezv/portucca`.

## Puesta en marcha (una vez)

1. Crear en GitHub `gomezv/portucca-internal` (privado) y `gomezv/portucca` (público).
2. En este repo: `git remote add origin https://github.com/gomezv/portucca-internal.git` y
   `git push -u origin main`.
3. Crear un PAT fine-grained con `Contents: Read and write` solo sobre `gomezv/portucca` y
   guardarlo como secret `PUBLIC_REPO_TOKEN` en este repo.
4. En el espejo público: `git remote add origin https://github.com/gomezv/portucca.git` y
   `git push -u origin main`.
5. A partir de ahí, cada merge a `main` en este repo sincroniza el público.

## Verificación

```bash
cd public
npm test
npm pack --dry-run
```

## Documentos internos

- [`docs/SPEC.md`](docs/SPEC.md)
- [`docs/DESIGN.md`](docs/DESIGN.md)
- [`docs/DECISIONS.md`](docs/DECISIONS.md)
- [`docs/ROADMAP.md`](docs/ROADMAP.md)
- [`docs/COMPETITORS.md`](docs/COMPETITORS.md)
- [`docs/PUBLISHING.md`](docs/PUBLISHING.md)
- [`docs/ADOPTION-SEUCCA.md`](docs/ADOPTION-SEUCCA.md)
