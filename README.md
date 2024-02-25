# Hoshigazeru: Mantenha-se Atualizado com Seu Anime Favorito!

**Hoshigazeru** é uma aplicação de servidor escrita em Go que se integra ao AniList para notificar você sobre novos episódios de seus animes favoritos. Cansado de perder lançamentos? Com o Hoshigazeru, você sempre saberá quando a próxima aventura te espera!

## Recursos

- Notificações em tempo real sobre novos episódios de anime.
- Usa GraphQL para buscar dados do AniList.
- Suporte a WebSocket para comunicação em tempo real.
- Suporte a Docker para fácil implantação.

## Como Executar

1. Clone o repositório.
2. Construa a imagem Docker usando o Dockerfile fornecido: `docker build -t hoshigazeru .`
3. Execute a imagem Docker: `docker run -p 8080:8080 hoshigazeru`

## Endpoints

### `/releases`

This is a WebSocket endpoint that sends real-time notifications about new anime episodes.

To connect to this endpoint, you need a WebSocket client. Once connected, the server will send a message whenever a new anime episode is released.

Example message:

```json
{
  "Id": 123,
  "Title": "Example Anime",
  "Episodes": 12,
  "AiringSchedule": [
    {
      "AiringAt": 1616055600,
      "Episode": 1
    }
  ],
  "Description": "This is an example anime.",
  "CoverImage": "https://example.com/image.jpg"
}
```

### `/animes`

This is an HTTP GET endpoint that returns a list of animes.

To use this endpoint, send a GET request to `https://kikyo.dev/animes`. The server will respond with a JSON array of animes.

Example response:

```json
[
  {
    "Id": 123,
    "Title": "Example Anime",
    "Episodes": 12,
    "AiringSchedule": [
      {
        "AiringAt": 1616055600,
        "Episode": 1
      }
    ],
    "Description": "This is an example anime.",
    "CoverImage": "https://example.com/image.jpg"
  },
  ...
]
```

## Contribuindo

Contribuições são bem-vindas! Sinta-se à vontade para enviar um Pull Request.