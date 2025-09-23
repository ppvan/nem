
/api/search?q=

-> return list of anime?
{
    id: int
    title: str,
    eps: []
}


Phan tich use case:
1. User want to search for anime to subscribe
2. User have a list of "subcribed" anime, they want to pick one from this
3. User want to add/remove one anime to subscribed list
4. User want to watch an anime right away


Use case:
As an user, I want to search for anime and subscribe to them.
The system must ensure I can watch any eps or the whole thing after subscribed




/api/anime/{anime_id}/{episode_id}/playlist.m3u8 -> The eps playlist
/api/anime/{anime-id}/playlist.m3u8 -> the "ALL" playlist
/api/popular
/api/recent
