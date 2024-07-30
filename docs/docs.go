// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/pools": {
            "get": {
                "description": "Returns a list of pools if the IDs parameter is not given. Otherwise,\nit batch fetches specific pools by the given pool IDs parameter.",
                "produces": [
                    "application/json"
                ],
                "summary": "Get pool(s) information",
                "operationId": "get-pools",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Comma-separated list of pool IDs to fetch, e.g., '1,2,3'",
                        "name": "IDs",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "List of pool(s) details",
                        "schema": {
                            "type": "array",
                            "items": {}
                        }
                    }
                }
            }
        },
        "/pools/canonical-orderbook": {
            "get": {
                "description": "Returns the canonical orderbook pool ID for the given base and quote.\nif the pool ID is not found for the given pair, it returns an error.\nif the base or quote denom are not provided, it returns an error.",
                "produces": [
                    "application/json"
                ],
                "summary": "Get canonical orderbook pool ID for the given base and quote.",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Base denom",
                        "name": "base",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Quote denom",
                        "name": "quote",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Canonical Orderbook Pool ID for the given base and quote",
                        "schema": {
                            "type": "struct"
                        }
                    }
                }
            }
        },
        "/pools/canonical-orderbooks": {
            "get": {
                "description": "Returns the list of canonical orderbook pool ID entries for all possible base and quote combinations.",
                "produces": [
                    "application/json"
                ],
                "summary": "Get entries for all supported orderbook base and quote denoms.",
                "responses": {
                    "200": {
                        "description": "List of canonical orderbook ool ID entries for all base and quotes",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/domain.CanonicalOrderBooksResult"
                            }
                        }
                    }
                }
            }
        },
        "/router/custom-direct-quote": {
            "get": {
                "description": "Call does not search for the route rather directly computes the quote for the given poolID.",
                "produces": [
                    "application/json"
                ],
                "summary": "Compute the quote for the given poolID",
                "operationId": "get-direct-quote",
                "parameters": [
                    {
                        "type": "string",
                        "example": "5OSMO",
                        "description": "String representation of the sdk.Coin for the token in.",
                        "name": "tokenIn",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "example": "ATOM,USDC",
                        "description": "String representing the list of the token denom out separated by comma.",
                        "name": "tokenOutDenom",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "example": "1,2,3",
                        "description": "String representing list of the pool ID.",
                        "name": "poolID",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "boolean",
                        "description": "Boolean flag indicating whether to apply exponents to the spot price. False by default.",
                        "name": "applyExponents",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "The computed best route quote",
                        "schema": {}
                    }
                }
            }
        },
        "/router/quote": {
            "get": {
                "description": "Returns the best quote it can compute for the exact in or exact out token swap method.",
                "produces": [
                    "application/json"
                ],
                "summary": "Optimal Quote",
                "operationId": "get-route-quote",
                "parameters": [
                    {
                        "type": "string",
                        "description": "String representation of the sdk.Coin denoting the input token for the exact amount in swap method.",
                        "name": "tokenIn",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "String representing the denomination of the output token for the exact amount in swap method.",
                        "name": "tokenOutDenom",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "String representation of the sdk.Coin denoting the output token for the exact amount out swap method.",
                        "name": "tokenOut",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "String representing the denomination of the input token for the exact amount out swap method.",
                        "name": "tokenInDenom",
                        "in": "query"
                    },
                    {
                        "type": "boolean",
                        "description": "Boolean flag indicating whether to return single routes (no splits). False (splits enabled) by default.",
                        "name": "singleRoute",
                        "in": "query"
                    },
                    {
                        "type": "boolean",
                        "description": "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally",
                        "name": "humanDenoms",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "boolean",
                        "description": "Boolean flag indicating whether to apply exponents to the spot price. False by default.",
                        "name": "applyExponents",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "The computed best route quote",
                        "schema": {}
                    }
                }
            }
        },
        "/router/routes": {
            "get": {
                "description": "returns all routes that can be used for routing from tokenIn to tokenOutDenom.",
                "produces": [
                    "application/json"
                ],
                "summary": "Token Routing Information",
                "operationId": "get-router-routes",
                "parameters": [
                    {
                        "type": "string",
                        "description": "The string representation of the denom of the token in",
                        "name": "tokenIn",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "The string representation of the denom of the token out",
                        "name": "tokenOutDenom",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "boolean",
                        "description": "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally",
                        "name": "humanDenoms",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "An array of possible routing options",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/sqsdomain.CandidateRoutes"
                            }
                        }
                    }
                }
            }
        },
        "/tokens/metadata": {
            "get": {
                "description": "returns token metadata with chain denom, human denom, and precision.\nFor testnet, uses osmo-test-5 asset list. For mainnet, uses osmosis-1 asset list.\nSee ` + "`" + `config.json` + "`" + ` and ` + "`" + `config-testnet.json` + "`" + ` in root for details.",
                "produces": [
                    "application/json"
                ],
                "summary": "Token Metadata",
                "operationId": "get-token-metadata",
                "parameters": [
                    {
                        "type": "string",
                        "description": "List of denoms where each can either be a human denom or a chain denom",
                        "name": "denoms",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Success",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "$ref": "#/definitions/domain.Token"
                            }
                        }
                    }
                }
            }
        },
        "/tokens/pool-metadata": {
            "get": {
                "description": "returns pool denom metadata. As of today, this metadata is represented by the local market cap of the token computed over all Osmosis pools.\nFor testnet, uses osmo-test-5 asset list. For mainnet, uses osmosis-1 asset list.\nSee ` + "`" + `config.json` + "`" + ` and ` + "`" + `config-testnet.json` + "`" + ` in root for details.",
                "produces": [
                    "application/json"
                ],
                "summary": "Pool Denom Metadata",
                "operationId": "get-pool-denom-metadata",
                "parameters": [
                    {
                        "type": "string",
                        "description": "List of denoms where each can either be a human denom or a chain denom",
                        "name": "denoms",
                        "in": "query"
                    },
                    {
                        "type": "boolean",
                        "description": "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally",
                        "name": "humanDenoms",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {}
            }
        },
        "/tokens/prices": {
            "get": {
                "description": "Given a list of base denominations, this endpoint returns the spot price with a system-configured quote denomination.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Get prices",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Comma-separated list of base denominations (human-readable or chain format based on humanDenoms parameter)",
                        "name": "base",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "boolean",
                        "description": "Specify true if input denominations are in human-readable format; defaults to false",
                        "name": "humanDenoms",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Specify the pricing source. Values can be 0 (chain) or 1 (coingecko); default to 0 (chain)",
                        "name": "pricingSource",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "A map where each key is a base denomination (on-chain format), containing another map with a key as the quote denomination (on-chain format) and the value as the spot price.",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "object",
                                "additionalProperties": {
                                    "type": "string"
                                }
                            }
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "domain.CanonicalOrderBooksResult": {
            "type": "object",
            "properties": {
                "base": {
                    "type": "string"
                },
                "contract_address": {
                    "type": "string"
                },
                "pool_id": {
                    "type": "integer"
                },
                "quote": {
                    "type": "string"
                }
            }
        },
        "domain.Token": {
            "type": "object",
            "properties": {
                "coingeckoId": {
                    "type": "string"
                },
                "decimals": {
                    "description": "Precision is the precision of the token.",
                    "type": "integer"
                },
                "preview": {
                    "description": "IsUnlisted is true if the token is unlisted.",
                    "type": "boolean"
                },
                "symbol": {
                    "description": "HumanDenom is the human readable denom.",
                    "type": "string"
                }
            }
        },
        "sqsdomain.CandidatePool": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "integer"
                },
                "tokenOutDenom": {
                    "type": "string"
                }
            }
        },
        "sqsdomain.CandidateRoute": {
            "type": "object",
            "properties": {
                "pools": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/sqsdomain.CandidatePool"
                    }
                }
            }
        },
        "sqsdomain.CandidateRoutes": {
            "type": "object",
            "properties": {
                "routes": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/sqsdomain.CandidateRoute"
                    }
                },
                "uniquePoolIDs": {
                    "type": "object",
                    "additionalProperties": {
                        "type": "object"
                    }
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "",
	BasePath:         "",
	Schemes:          []string{},
	Title:            "Osmosis Sidecar Query Server Example API",
	Description:      "",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
