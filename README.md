# ALPHA: Closed-Loop Autonomic Cyber Defence and Recovery for Maritime CPS

This repository contains the inspectable research artefacts, theoretical data models, and partial source code for the paper **"Closed-Loop Autonomic Cyber Defence and Recovery for Maritime Cyber-Physical Systems via Dynamic Knowledge-Graph-to-PDDL Planning"**.

## Abstract & Artefact Scope

ALPHA is a closed-loop autonomic recovery architecture predicated on the MAPE-K reference model, customised specifically for maritime cyber-physical systems (CPS). The framework dynamically compiles the real-time operational state of a vessel&mdash;maintained within a Knowledge Graph (KG)&mdash;into a Planning Domain Definition Language (PDDL) problem instance. Utilising the Fast Downward planner, ALPHA synthesises cost-optimal recovery plans that adhere to rigorous physical and navigational safety constraints. 

### Open Science and Artefact Availability Statement

In alignment with the Open Science policy of ACM CCS 2026, this repository has been structured to maximise the transparency, reproducibility, and long-term impact of our research, whilst carefully balancing critical industrial and security constraints. All materials contained within this research project are officially released under the **Creative Commons Attribution-NonCommercial-NoDerivatives 4.0 International (CC BY-NC-ND 4.0)** licence.

**Justification for Partial Redaction:**
The ALPHA framework represents a highly industry-relevant prototype designed for critical maritime infrastructure. Consequently, providing a fully executable, end-to-end replication package&mdash;specifically, the deployable binaries and proprietary runtime containers for the `StateManager` and `Executor`&mdash;is neither legally nor practically feasible. These components encapsulate restricted industrial intellectual property and overarching integration logic. Furthermore, releasing a fully automated, production-ready cyber defence orchestrator presents responsible disclosure concerns and potential deployment risks if such adversarial countermeasures are released unconditionally. 

**Scope of the Inspectable Artefact:**
To ensure our core scientific claims remain thoroughly evaluable, and in strict adherence to the ACM guideline encouraging the provision of partial or synthetic artefacts when full sharing is precluded, this repository serves as a comprehensive **Inspectable Artefact**. It provides complete transparency into the theoretical and methodological core of the paper, including:

*   **Theoretical Models:** The complete PDDL domain definitions and formal Knowledge Graph ontologies.
*   **Algorithmic Transparency:** The partial source code exposing the inner mapping logic of the `Planner` and the Graph-to-PDDL translation mechanisms.
*   **Evaluation Methodology:** The complete suite of synthetic test orchestrators, benchmarking scripts (`load_test.js`), and datasets utilised to validate the infrastructural thresholds of the proposed architecture.

This structured approach guarantees that reviewers and the broader academic community can rigorously assess the validity of the methodology, the dynamic penalty assignments, and the synthesised recovery plans, without necessitating access to restricted, proprietary execution environments.

## Repository Structure

The directory tree is organised to map directly to the conceptual components detailed in the paper:

*   **`assets/`**
    *   `domain.pddl`: The core, generalised PDDL domain model. It formally encodes all admissible interventions (e.g., host containment, service failover, network isolation), physical safety constraints, and the context-dependent cost optimisation model.
*   **`docs/`**
    *   `schema.json`: The formal ontology of the maritime domain. It delineates the graph nodes (e.g., `Service`, `Capability`) and the valid-time temporal semantics requisite for tracking systemic state evolution.
    *   `phi.json`: The exhaustive graph dump of the PHI testbed infrastructure. It exposes the segmented maritime network topology and the device inventory discussed within the paper's evaluation.
*   **`examples/`**
    *   Contains paired examples of dynamically generated PDDL problem instances (`*.pddl`) and their corresponding cost-optimal solution plans (`*.plan`) synthesised by Fast Downward. These instances demonstrate how risk-derived goals and structural dependencies are translated at runtime.
*   **`internal/planner/`**
    *   The open-sourced Go routines of ALPHA's planning layer. This directory exposes the proprietary translation algorithms (e.g., `problem_space.go`, `encoder.go`) responsible for the `EXTRACT_PROBLEM_SPACE` procedure detailed in Section 6 of the paper. It additionally includes the specific tactical implementations (`strategy_contain.go`, `strategy_reconfig.go`, etc.) that assign dynamic costs and penalties.
*   **`pkg/`**
    *   `message.go`: The explicit data contracts defining the Input/Output JSON structures expected over the Redis streams (Event, Alert, and Plan streams), thereby ensuring the architectural transparency of the decoupled MAPE-K loop.
*   **`tests/`**
    *   The comprehensive test orchestration harness employed to rigorously evaluate the infrastructural thresholds of the proposed architecture. This encompasses the complete suite of service adapters and benchmarking scripts utilised to validate the performance, resilience, and scalability of the entire system under simulated load.
*   **`docker-compose.yml`**
    *   Provided strictly as structural documentation to demonstrate how the test orchestrator and underlying databases (Redis, Memgraph, InfluxDB, Grafana) are topologically provisioned during laboratory experiments.

## Artefact Inspection Guide

Whilst researchers cannot execute an end-to-end simulation of the response loop, the methodology may be rigorously audited:

1.  **AI Planning Methodology**: Review `assets/domain.pddl` in conjunction with the files situated in `examples/` to verify the logic of the dynamic penalty assignments and the synthesised recovery sequences.
2.  **State Representation**: Inspect `docs/schema.json` and `docs/phi.json` to comprehend how complex maritime IT/OT dependencies and redundancies are structurally modelled as a labelled property multigraph.
3.  **Graph-to-PDDL Translation**: Examine the Go source code within `internal/planner/` to audit how ALPHA traverses the Knowledge Graph and translates the minimal blast radius into actionable planning constraints.
4.  **Performance Evaluation Methodology**: Audit `tests/load_test.js` and the associated `adapters/` to validate the stringent stress-testing methodology applied to the data processing pipeline.