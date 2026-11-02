# Building the emoji catalogue

The source manifest pins Twemoji 17.0.3, Unicode Emoji 17.0 and CLDR 48.1. The seeder verifies each data-file checksum and a checksum over the entire SVG tree before constructing a complete catalogue. It keeps complete Unicode sequences and checks all required English and Chinese metadata before any database transaction. Three reviewed Chinese names supplement CLDR; one reviewed description improves the party-popper search. Licenses and attribution are embedded in `api/emojilib/licenses` and exposed by the local API.

From `shared/go`:

```sh
sh cmd/emoji-seed/fetch-sources.sh /tmp/emoji-sources
go run ./cmd/emoji-seed --input /tmp/emoji-sources --output /tmp/emoji-catalogue.json
# Set standard PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE and PGSSLMODE first.
go run ./cmd/emoji-seed --input /tmp/emoji-sources --import
```

Apply `ChenWeb/shared_migrations/20261103000002_create_shared_emoji_library.sql` through the shared goose migration track before import. The target database must provide ParadeDB `pg_search` with the `jieba` tokenizer. The importer runs one transaction and does not delete earlier sequences. A failed validation or transaction preserves the previous catalogue; running unchanged inputs again leaves row versions unchanged.

The checked-in source pins are intentional: updating Unicode, CLDR or artwork requires updating the manifest and reviewing coverage, missing Chinese translations, license terms and the category dictionary. Missing metadata fails the build and identifies the Unicode IDs. Supply reviewed translations with `--overrides FILE` in JSON, for example `{"keywords_zh":{"1f389":"庆祝 礼花"},"names_zh":{},"descriptions_zh":{}}`; retain that file with the matching source manifest. Do not substitute English keywords for Chinese. Never download assets during application startup or while serving a request.
