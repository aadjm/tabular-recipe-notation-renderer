# testdata — fixtures and goldens

Every `<name>.trn.md` here is a TRN document; `<name>.table.html` and
`<name>.flow.html` are its committed renderings of the two views, and
`chai.doc.html` is a whole standalone page (`RenderDocument`) for the canonical
chai fixture — plain HTML, no stylesheet, both views in order. `go test ./...`
compares the renderers against those files;
`go test ./... -update` rewrites them (SPEC-ENGINE §6).

## Fixtures

| fixture | origin |
| --- | --- |
| `chai.trn.md` | SPEC-ENGINE canonical example (single-line steps) |
| `guacamole.trn.md` | SPEC-ENGINE canonical example (continuation lines) |
| `kraft-dinner.trn.md` | Kraft Dinner example from TheSonOfThomp/recipe-parser, verbatim |
| `grocy-1.trn.md` … `grocy-5.trn.md` | the `trn` userfield of recipes 1–5 on the live Grocy instance |

The `grocy-*.trn.md` files are **byte-identical** to what Grocy stores: no
normalization, no re-indent, and no trailing newline added. Recipes 1–3 have no
final newline in the userfield and neither do their files; recipes 4–5 end with
one and so do theirs. Going through `Parse` therefore round-trips the exact text
the site will hand the engine, trailing-newline edge cases included. Do not
"tidy" these files by hand — `gofmt`/editors that append a newline will break the
byte-exactness that makes them fixtures rather than transcriptions.

Recipe ids are Grocy's. Id 4 is named "Pizza" in Grocy while its TRN title line
says "Margherita Pizza"; Grocy's name is canonical for an index, the TRN title
wins on a recipe page, so both are expected and the fixture keeps the TRN title.

## Refreshing the Grocy fixtures

The API key lives in `/root/.env` (`GROCY_API_KEY`) and must never be committed
— see SPEC-SITE §5 and the `grocy-inventory` skill. The instance is reached over
the LAN at `GROCY_URL` (currently `http://192.168.1.196:9283`), not through the
`*.samizdat.site` SSO hostname, which rejects API-key requests.

```
GET $GROCY_URL/api/userfields/recipes/<id>
  with the request header GROCY-API-KEY set from GROCY_API_KEY in /root/.env
```

Write the returned `trn` string out with no added bytes, then regenerate the
goldens and eyeball the diff before committing:

```
go test ./... -update
go test ./...
```

A fixture that parses with warnings fails `loadRecipe` on purpose: every
priority fixture is expected to be clean, so a warning means either the recipe
text or the parser changed and one of them deserves a look.

## Provenance

Fetched 2026-10-17 from the live instance; SHA-256 of each file (equal to the
SHA-256 of the stored userfield string):

```
5e32e14b790e46793005454eaba9ee5a53921bc2012ce7c6ca2c37e8be6d3458  grocy-1.trn.md   Chicken Curry
2b869d980ae1edb8c86aa138b68a2d0099a91a969280e1490b4cd875a2cc5040  grocy-2.trn.md   Guac
d641deb79e821b5db2036c68033c71dd9bff3f8f1e50f847e2d243eec45a106e  grocy-3.trn.md   Kraft
143e96878d6016fa550a10fb98219d9e7f9303895ae6e53ecb9354d470526a83  grocy-4.trn.md   Pizza / "Margherita Pizza"
794582122773c6937726df00e67024104e575a5582887612bfb79aed7f6480a8  grocy-5.trn.md   Masala Chai
```

If a recipe is edited in Grocy, refetch and update the digest here along with the
goldens; a mismatch between file and `GET /api/userfields/recipes/<id>` is a
fixture bug, not a rendering bug.
