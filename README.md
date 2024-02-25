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

- `/releases`: Endpoint WebSocket que envia notificações em tempo real sobre novos episódios de anime.
- `/animes`: Endpoint HTTP GET que retorna uma lista de animes.

## Contribuindo

Contribuições são bem-vindas! Sinta-se à vontade para enviar um Pull Request.