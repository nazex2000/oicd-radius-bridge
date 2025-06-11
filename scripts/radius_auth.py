#!/usr/bin/env python3

import json
import sys
import requests
from typing import Dict, Any

# API Configuration
API_URL = "http://localhost:8080/auth" #TODO: Change to the actual API URL

def authenticate(username: str, password: str) -> bool:
    """
    Authenticates the user through the OIDC-RADIUS Bridge API.
    
    Args:
        username: Username or email
        password: User password
        
    Returns:
        bool: True if authentication was successful, False otherwise
    """
    try:
        response = requests.post(
            API_URL,
            json={"username": username, "password": password},
            headers={"Content-Type": "application/json"},
            timeout=5
        )
        
        if response.status_code == 200:
            return True
        return False
        
    except Exception as e:
        print(f"Authentication error: {str(e)}", file=sys.stderr)
        return False

def main() -> None:
    """
    Main function that processes RADIUS authentication.
    """
    if len(sys.argv) != 3:
        print("Usage: radius_auth.py <username> <password>", file=sys.stderr)
        sys.exit(1)
        
    username = sys.argv[1]
    password = sys.argv[2]
    
    if authenticate(username, password):
        sys.exit(0)
    else:
        sys.exit(1)

if __name__ == "__main__":
    main() 
    