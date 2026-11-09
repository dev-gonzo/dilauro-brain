# Brain

Stack local que centraliza a inteligência de conteúdo e, futuramente, automações de atendimento.

| Serviço | Porta local | Papel |
| --- | ---: | --- |
| API Brain (Go) | `47180` | Entrada do painel e health consolidado |
| Processador IA (Python) | `47181` | Tratamento de dados e tarefas de IA |
| PostgreSQL | `47182` | Persistência local |

Todas as portas são vinculadas a `127.0.0.1`; não ficam acessíveis pela rede local. Entre os containers, a comunicação usa a rede Docker `brain-network`.

## Iniciar

1. Copie `.env.example` para `.env` e substitua `POSTGRES_PASSWORD` por uma senha segura.
2. Na pasta `brain`, execute `docker compose up --build -d`.
3. Consulte `GET http://127.0.0.1:47180/health` a partir do painel administrativo.

Exemplo de resposta saudável:

```json
{
  "status": "ok",
  "services": {
    "go": { "status": "ok" },
    "python": { "status": "ok" },
    "database": { "status": "ok" }
  }
}
```

O endpoint retorna `200` apenas quando os três componentes estão saudáveis; em caso contrário retorna `503`, preservando o estado individual para exibição no painel.

## Encerrar

```sh
docker compose down
```

Use `docker compose down -v` somente se quiser apagar o banco de dados local.
