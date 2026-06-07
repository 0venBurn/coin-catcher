# Std lib
import asyncio
import json
import time

# External
import httpx

# Project Imports
from src import config
from src.api_response_types import (
    CommodityAuction,
    CommodityAuctionsApiResponse,
    OAuthTokenResponse,
)


async def get_token(client: httpx.AsyncClient) -> tuple[str, float]:
    response = await client.post(
        url=config.OAUTH_TOKEN_URL,
        data={"grant_type": "client_credentials"},
        auth=(config.CLIENT_ID, config.CLIENT_SECRET),
    )
    response.raise_for_status()
    data: OAuthTokenResponse = response.json()
    return data["access_token"], time.time() + data["expires_in"]


async def get_commodities(
    client: httpx.AsyncClient,
    region_url: str,
    endpoint: str,
    namespace: str,
    token: str,
) -> list[CommodityAuction]:
    url = f"https://{region_url}{endpoint}"
    headers = {"Authorization": f"Bearer {token}"}
    params = {"namespace": namespace}

    response = await client.get(url=url, headers=headers, params=params)
    response.raise_for_status()
    data: CommodityAuctionsApiResponse = response.json()
    return data["auctions"]


async def main():
    async with httpx.AsyncClient() as client:
        token, _ = await get_token(client)
        region_info = config.BLIZZARD_URL_REGIONS_INFO["us"]
        commodities = await get_commodities(
            client,
            region_info["url"],
            config.BLIZZARD_ENDPOINTS["commodities"],
            region_info["namespace"],
            token,
        )
        print(json.dumps(commodities))


if __name__ == "__main__":
    asyncio.run(main())
