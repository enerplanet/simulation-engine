# EnerPlanET and Docker Webservice

## Introduction

EnerPlanET is an advanced application designed for energy simulation and analysis, utilizing powerful tools like Calliope and PyPSA. The application leverages a service called "Docker Webservice" to run these simulations in a containerized environment, ensuring scalability, consistency, and ease of deployment. This documentation provides an overview of how EnerPlanET interacts with the Docker Webservice, including the structure of the JSON input and the workflow for energy simulations.

## Overview of Docker Webservice

The Docker Webservice is a key component of EnerPlanET, providing a containerized environment to run energy simulations using Calliope and PyPSA. It ensures that all dependencies and configurations are managed within Docker containers, making the deployment and execution of complex simulations both reliable and reproducible.

## Core Components and Technologies

- **Calliope**: A versatile energy system modeling framework that allows for the creation and analysis of various energy scenarios, including renewable energy integration and demand-response strategies.

- **PyPSA**: Python for Power System Analysis (PyPSA) is an open-source toolbox for simulating and optimizing modern electrical power systems. It allows EnerPlanET to model and analyze power flow, load balancing, and other critical aspects of energy networks.

- **Docker**: The simulations are executed within Docker containers, which encapsulate all necessary software, libraries, and configurations. This ensures that the simulations run in a consistent environment, regardless of the underlying system.

## JSON Structure and Workflow

EnerPlanET interacts with the Docker Webservice using structured JSON input, which specifies the parameters for the energy simulations. Below is an explanation of the key elements of the JSON structure used in EnerPlanET:

### 1. User Information

- **`user_id`**: Identifies the user requesting the simulation.
- **`model_id` and `session_id`**: Unique identifiers for the simulation model and session, ensuring that results can be tracked and managed effectively.

### 2. Simulation Parameters

- **`lkr`**: Represents the location or region for the simulation, e.g., "Deggendorf".
- **`start_date` and `end_date`**: Define the timeframe for the energy simulation.
- **`resolution`**: Specifies the time resolution of the simulation in minutes (e.g., 60 minutes).

### 3. Custom Demand Time Series

- **`custom_demand_time_series`**: An array where users can specify custom demand profiles. This is particularly useful for simulating specific energy usage patterns.

### 4. Topology

The topology section defines the energy infrastructure, including nodes and connections, using a geographical and technical representation.

- **`from` and `to`**: Define the start and end points of connections within the energy network. These are represented as geographical points with additional properties, such as `feature_type` and `slp_class`.
- **`length`**: Specifies the physical length of the connection, which may affect power losses and other parameters.
- **`pipe`**: Indicates the type of connection (e.g., "lv" for low voltage).

**Example**: 

- **`wind_supply`**: Represents a wind energy source with various constraints and costs, such as `cont_energy_cap_max` (maximum energy capacity) and `cost_energy_cap` (capital cost per unit of energy).

### 5. PyPSA Configuration

- **`pypsa`**: This section includes parameters specific to the PyPSA simulations, such as transformer types (`trafo_mv_lv_type`) and line types (`line_type_mv`, `line_type_lv`). These settings are crucial for accurately modeling the electrical network and its components.

### 6. Callback URL

- **`callback_url`**: The URL to which the results of the simulation will be sent once the Docker Webservice completes the process. This allows for seamless integration with other systems or user interfaces.

## Simulation Workflow

1. **Input Preparation**: The user inputs the necessary data through the EnerPlanET interface, which is then structured into a JSON format.

2. **Service Request**: The JSON is sent to the Docker Webservice, which triggers the simulation. This service runs the simulation using the Calliope and PyPSA frameworks within Docker containers.

3. **Simulation Execution**: Within the Docker containers, the simulation is executed based on the provided parameters. The container ensures a consistent environment, so results are reproducible and reliable.

4. **Result Handling**: Upon completion, the results are sent back to EnerPlanET via the `callback_url`, where they can be processed and displayed to the user.

5. **Post-Simulation**: Users can review the results, make adjustments, and potentially run further simulations to refine their energy models.

## Conclusion

EnerPlanET, supported by the Docker Webservice, provides a robust platform for conducting advanced energy simulations using Calliope and PyPSA. By leveraging the power of containerization, EnerPlanET ensures that simulations are consistent, scalable, and easy to deploy, enabling users to model and analyze complex energy systems effectively. The JSON-based workflow allows for flexible and detailed configurations, ensuring that simulations are tailored to specific user needs and regional requirements.

### Detail Documentation

For Installation Instruction, please visit the project [wiki](https://mygit.th-deg.de/tcf_s6et/docker_webservice/-/wikis/home).
