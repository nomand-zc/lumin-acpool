English | [中文](./docs/README_zh_CN.md)

## LUMIN

Light up AI routing. Hide the complexity.

### Introduction

**LUMIN** is a lightweight, unified AI proxy SDK designed for multi-platform model calls, account pool management, and intelligent routing.

It uniformly encapsulates and completely hides the protocol differences of various AI platforms such as Kiro, GeminiCLI and Codex, and provides a consistent, simple, and stable calling interface to the outside world. This allows upper-layer businesses to focus on their core logic without caring about the underlying platform details, perfectly embodying the core concept of "Cloud Hiding" — hiding complexity in the bottom layer and leaving simplicity for businesses.

### Core Capabilities

- 🌥 **Cloud Hiding Architecture** : Shield protocol differences of various AI vendors, provide a unified standard interface, and eliminate the need for business adaptation to multiple platforms

- 🧠 **Intelligent Account Pool** : Built-in multi-account management, load balancing, automatic account selection, and fault elimination to ensure proxy stability

- 🔗 **Unified Proxy Client** : One-time access to support calls from multiple platforms such as Kiro, Gemini and Codex

- ⚡ **High Availability & Transparent Routing** : Dynamic routing, automatic retry, current limiting protection, and seamless switching to reduce business exception rates

- 📦 **Lightweight SDK** : Non-intrusive, easy to integrate, can be directly embedded into projects as a dependency library without additional deployment costs

### Application Scenarios

- Unified calling of multi-platform AI models to avoid repetitive development of adaptation code

- AI platform account pool management and intelligent account selection to improve call success rate

- Decoupling between business layer and third-party AI platforms to reduce platform switching costs

- Backend services that require a high-stability, low-latency AI proxy gateway

- Cloud-native projects that hope to "hide complexity" and focus on business logic development

### Technical Features

- Written in pure Golang, with high performance and low memory usage, suitable for backend service scenarios

- No dependency on intermediate services, used directly as an SDK with simple deployment

- Extensible architecture, new AI platform access only requires developing an adaptation layer with extremely low cost

- Built-in retry, circuit breaking, timeout, and health check mechanisms to improve service availability

- Simple configuration and concise API design, enabling developers to get started and integrate quickly

### Project Positioning

**LUMIN = Cloud Hiding · Unified AI Proxy Gateway**

Let businesses focus only on logic, not platforms; let complexity be hidden, and calls be simpler.
