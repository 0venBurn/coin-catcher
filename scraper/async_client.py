"""
Async HTTPX client for Blizzard World of Warcraft API.

Key concepts demonstrated:
- AsyncClient with connection pooling (reuse for multiple requests)
- Proper async/await patterns
- Error handling with retries
- Context manager usage for cleanup
- Type hints for better IDE support
"""

from __future__ import annotations

import os

import asyncio

import httpx
# Configuration - these would come from environment variables
OAUTH_TOKEN_URL = "https://oauth.battle.net/token"
CLIENT_ID = os.getenv("CLIENT_ID")  
CLIENT_SECRET = os.environ["CLIENT_SECRET"]  

class Person:
    def __init__(self, name, address) -> None:
        self.name = name
        self.address = address 
        


async def get_token(client_id: str, client_secret: str) -> str: 
    async with httpx.AsyncClient() as client:
        response = await client.post(
                OAUTH_TOKEN_URL, 
                data={"grant_type": "client_credentials"}, 
                auth=(client_id, client_secret))
        return response.json()


async def main():
    token = await get_token(CLIENT_ID, CLIENT_SECRET)
    print(f"got token {token}")

if __name__ == "__main__":
    asyncio.run(main())
