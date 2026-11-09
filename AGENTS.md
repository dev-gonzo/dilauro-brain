# Brain

- Use versionamento SemVer para os serviços Go e Python. A versão atual inicia em `1.0.0` e deve ser atualizada nos endpoints `GET /version` de cada serviço.
- O banco é versionado exclusivamente por migrações Flyway em `migrations/`, com nomes como `V1_0_0__descricao.sql`. Nunca altere uma migração já aplicada; crie a próxima versão.
- A API Go é a única interface do painel administrativo com o Brain. Ela agrega o health, a versão do Python e a versão da última migração do banco. O Admin nunca se comunica diretamente com o Python ou Postgres.
