#!/usr/bin/env python3
"""
Hyperliquid EIP-712 signing helper for EvoClaw Rust agent.
Usage: python hl_sign.py <wallet_address> <private_key> <action_json>
"""

import sys
import json
from eth_account import Account
from eth_account.messages import encode_defunct, encode_structured_data

def sign_hyperliquid_action(wallet_address: str, private_key: str, action: dict) -> dict:
    """
    Sign a Hyperliquid action using EIP-712 structured data signing.
    
    Args:
        wallet_address: Ethereum wallet address (0x...)
        private_key: Private key for signing
        action: Action dictionary (order, cancel, etc.)
    
    Returns:
        Signed action with r, s, v signature components
    """
    # EIP-712 domain for Hyperliquid
    domain = {
        "name": "Exchange",
        "version": "1",
        "chainId": 1337,  # Hyperliquid uses 1337 for mainnet
        "verifyingContract": "0x0000000000000000000000000000000000000000"
    }
    
    # Determine message type based on action
    action_type = action.get("type", "order")
    
    if action_type == "order":
        message_types = {
            "Order": [
                {"name": "asset", "type": "uint32"},
                {"name": "isBuy", "type": "bool"},
                {"name": "limitPx", "type": "uint64"},
                {"name": "sz", "type": "uint64"},
                {"name": "reduceOnly", "type": "bool"},
                {"name": "timestamp", "type": "uint64"},
            ]
        }
        message = {
            "asset": action["asset"],
            "isBuy": action["isBuy"],
            "limitPx": action["limitPx"],
            "sz": action["sz"],
            "reduceOnly": action.get("reduceOnly", False),
            "timestamp": action["timestamp"],
        }
    elif action_type == "cancel":
        message_types = {
            "Cancel": [
                {"name": "asset", "type": "uint32"},
                {"name": "oid", "type": "uint64"},
            ]
        }
        message = {
            "asset": action["asset"],
            "oid": action["oid"],
        }
    else:
        raise ValueError(f"Unknown action type: {action_type}")
    
    # Encode structured data
    structured_data = {
        "types": {
            "EIP712Domain": [
                {"name": "name", "type": "string"},
                {"name": "version", "type": "string"},
                {"name": "chainId", "type": "uint256"},
                {"name": "verifyingContract", "type": "address"},
            ],
            **message_types
        },
        "primaryType": list(message_types.keys())[0],
        "domain": domain,
        "message": message
    }
    
    encoded = encode_structured_data(structured_data)
    
    # Sign with private key
    account = Account.from_key(private_key)
    signed = account.sign_message(encoded)
    
    return {
        "r": hex(signed.r),
        "s": hex(signed.s),
        "v": signed.v
    }

def main():
    if len(sys.argv) != 4:
        print("Usage: python hl_sign.py <wallet_address> <private_key> <action_json>", file=sys.stderr)
        sys.exit(1)
    
    wallet_address = sys.argv[1]
    private_key = sys.argv[2]
    action_json = sys.argv[3]
    
    try:
        action = json.loads(action_json)
        signature = sign_hyperliquid_action(wallet_address, private_key, action)
        print(json.dumps(signature))
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
