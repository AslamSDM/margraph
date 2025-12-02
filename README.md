# Margraf: Financial Dynamic Knowledge Graph (FDKG)

This program implements a prototype for a Financial Dynamic Knowledge Graph as described in the research. It models the global economy as a graph of Nations, Products, and Corporations, linked by Trade and Capital flows.

## Features

- **Graph Core**: Nodes (Nations, Products) and Edges (Trade flows).
- **AI-Powered Initialization**: Uses LLMs (Google Gemini) to discover major economic entities and trade relationships.
- **Shock Simulation**: Simulates economic shocks (e.g., Trade Bans) and propagates them to see direct and ripple effects.
- **Robustness**: Fallback to mock data if API keys are missing or APIs are unreachable.

## Prerequisites

- Go 1.18+
- (Optional) Google Gemini API Key for dynamic discovery.

## Setup

1.  Initialize the module:
    ```bash
    go mod tidy
    ```

2.  (Optional) Set your API Key:
    ```bash
    export GEMINI_API_KEY="your_api_key_here"
    ```

3.  (Optional) Configure the AI Model (defaults to gemini-1.5-flash):
    ```bash
    export GEMINI_MODEL="gemini-1.5-pro"
    ```

## Running

Build and run the application:

```bash
go build -o margraf_app
./margraf_app
```

## Usage

Once running, the CLI accepts commands:

- `show`: Displays the current graph (nodes and edges).
- `shock <node_id>`: Simulates a shock on a specific node.
    - Example: `shock india` (Simulates a trade ban on India).
- `exit`: Quits the program.

## Architecture

- `graph/`: Core data structures (Graph, Node, Edge).
- `discovery/`: Seeder logic that uses LLM to populate the graph.
- `simulation/`: Logic for propagating shocks through the graph.
- `llm/`: Client for interacting with Generative AI models.
