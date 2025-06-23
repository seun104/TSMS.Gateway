# Technical Documentation: Arad SMS Gateway

## 1. Introduction

This document provides a technical overview of the Arad SMS Gateway application. The system is a comprehensive SMS gateway solution built primarily using .NET technologies. It features a web-based UI for administration and user interaction, a public HTTP API for external integrations, a robust data layer, a suite of background services for asynchronous processing, and integration with multiple SMS providers.

## 2. System Architecture

The Arad SMS Gateway follows a multi-layered architecture:

*   **User Interface (UI)**: Provides the web portal for users and administrators.
*   **API Layer**: Exposes functionalities to external applications.
*   **Business Logic Layer**: Encapsulates the core business rules and operations.
*   **Data Access Layer (DAL)**: Handles all database interactions.
*   **Background Workers**: Perform asynchronous tasks like message sending, status updates, and logging.
*   **SMS Provider Integration Layer**: Manages communication with various SMS service providers.
*   **Shared Libraries**: Provide common utilities and functionalities across the application.

## 3. Core Modules

### 3.1. API (Arad.SMS.Gateway.WebApi)

*   **Purpose**: Provides a public HTTP API for interacting with the SMS gateway.
*   **Location**: `src/Api/Arad.SMS.Gateway.WebApi/`
*   **Functionality**:
    *   Exposes endpoints for sending SMS, retrieving SMS status, managing phonebooks, receiving SMS, etc.
    *   Handles authentication (IP-based, token-based) and authorization.
    *   Uses OWIN for self-hosting the Web API, allowing it to run as a Windows Service (managed by Topshelf).
    *   Supports request/response logging and exception handling through custom handlers and attributes.
    *   Configuration is loaded from `Configurations/Configortion.config`.
    *   Supports localization (English and Farsi).
*   **Key Technologies**: ASP.NET Web API, OWIN, Topshelf, C#.

### 3.2. Background Workers

*   **Purpose**: Perform various asynchronous tasks essential for the gateway's operation, typically implemented as Windows Services.
*   **Location**: `src/BackgroundWorker/`
*   **Key Workers**:
    *   **`Arad.SMS.Gateway.ApiProcessRequest`**: Processes API requests asynchronously using a dedicated thread (`APIThread`).
    *   **`Arad.SMS.Gateway.GetSmsDelivery`**: Retrieves SMS delivery statuses from providers using a `DeliveryThread`. Integrates with external provider web services (e.g., Asanak).
    *   **`Arad.SMS.Gateway.ExportData`**: Handles data export functionalities.
    *   **`Arad.SMS.Gateway.GiveBackCredit`**: Manages credit refund processes.
    *   **`Arad.SMS.Gateway.MessageParser`**: Parses incoming SMS messages based on defined rules.
    *   **`Arad.SMS.Gateway.RegularContent`**: Manages sending of scheduled or recurring SMS content.
    *   **`Arad.SMS.Gateway.SaveLog`**: Persists application and service logs.
    *   **`Arad.SMS.Gateway.SaveSentSms`**: Archives details of sent SMS messages.
    *   **`Arad.SMS.Gateway.SaveSmsDelivery`**: Saves delivery reports fetched from providers.
    *   **`Arad.SMS.Gateway.TerraficRelay`**: Relays SMS traffic, possibly for routing or load balancing.
*   **Common Features**:
    *   Designed to run as Windows Services.
    *   Utilize multi-threading for concurrent operations.
    *   Many include a `GarbageCollectorThread` for explicit memory management in long-running services.
    *   Depend on the Data Layer and General Library for core functionalities.
*   **Key Technologies**: .NET Framework (Windows Services), C#.

### 3.3. Data Layer

*   **Purpose**: Manages all data persistence and retrieval operations.
*   **Location**: `src/DataLayer/`
*   **Components**:
    *   **`Arad.SMS.Gateway.Common`**: Contains POCOs/DTOs representing application entities (e.g., `User`, `Outbox`, `PhoneBook`).
    *   **`Arad.SMS.Gateway.DataAccessLayer`**:
        *   Provides the core data access logic using ADO.NET (`System.Data.SqlClient`) for direct SQL Server interaction.
        *   The `DataAccess.cs` class acts as a base or utility for CRUD operations and stored procedure execution.
        *   Manages database connections and transactions.
    *   **`Arad.SMS.Gateway.Business`**: Contains business logic classes that operate on the common entities and utilize the `DataAccessLayer`.
    *   **`Arad.SMS.Gateway.Facade`**: Implements the Facade pattern, providing a simplified static interface to the business logic and data access operations for other modules.
*   **Key Technologies**: ADO.NET, C#, SQL Server.

### 3.4. Libraries and Tools

*   **Purpose**: Provide reusable utilities and functionalities.
*   **Location**: `src/LibraryAndTools/`
*   **Components**:
    *   **`Arad.SMS.Gateway.GeneralLibrary`**:
        *   A comprehensive utility library with helpers for encryption (`CryptorEngine`), date/time manipulation (`DateManager`), logging (`LogController`), configuration management (`ConfigurationManager`), online payment integration (Mellat, Parsian), serialization, and base classes for entities.
    *   **`Arad.SMS.Gateway.GeneralTools`**: Contains custom UI controls (e.g., DataGrid, SearchBox) likely for the ASP.NET WebForms UI.
    *   **`Arad.SMS.Gateway.SqlLibrary`**: Contains SQL-specific helper classes, including queue management and direct SQL execution utilities (`SQLHelper`).
*   **Key Technologies**: .NET Framework, C#.

### 3.5. SMS Provider Workers

*   **Purpose**: Manage interactions with different SMS service providers.
*   **Location**: `src/SMSProviderWorker/`
*   **Components**:
    *   Individual projects for each provider (e.g., `AradSmsSender`, `MagfaSmsSender`, `RahyabPGSmsSender`, `RahyabRGSmsSender`). Each is a Windows Service.
    *   Each provider worker contains a specific `ServiceManager` (e.g., `AradSmsServiceManager.cs`) to handle the provider's API and a sending thread (e.g., `AradThread.cs`).
    *   **`Arad.SMS.Gateway.ManageThread`**: A common library used by provider workers for managing sending threads (`WorkerThread`) and garbage collection.
*   **Functionality**: Each worker connects to its respective SMS provider, sends messages, and may handle delivery report retrieval if not covered by `GetSmsDelivery` worker.
*   **Key Technologies**: .NET Framework (Windows Services), C#.

### 3.6. User Interface (UI)

*   **Purpose**: Provides the web-based user interface for managing the SMS gateway.
*   **Location**: `src/UI/`
*   **Components**:
    *   **`Arad.SMS.Gateway.Web`**:
        *   The main web application built with ASP.NET WebForms.
        *   `Global.asax.cs` handles application lifecycle events, session initialization (language, encryption), and response compression.
        *   `PageLoader.aspx` dynamically loads user controls (`.ascx`) to render pages, enabling a modular UI structure.
        *   Extensive use of user controls (`UI/**/*.ascx`) for various features like user management, SMS sending, reporting, and settings.
        *   Integrates various client-side libraries: jQuery, Bootstrap, CKEditor, EasyUI, and others for rich user interactions.
        *   `CometService/CometAsyncHandler.ashx` suggests implementation of real-time features.
    *   **`Arad.SMS.Gateway.URLRewriter`**: Handles URL rewriting for user-friendly URLs.
*   **Key Technologies**: ASP.NET WebForms, C#, HTML, CSS, JavaScript (jQuery, Bootstrap, EasyUI, CKEditor).

### 3.7. Database (DB)

*   **Purpose**: Stores all persistent data for the application.
*   **Location**: `DB/`
*   **Contents**:
    *   `Arad.SMS.Gateway.DB.sql`: SQL script for database schema (tables, views, stored procedures).
    *   `Arad.SMS.Gateway.DB_Data.sql`: SQL script for initial/master data population.
*   **Key Tables (Probable)**: `Users`, `Roles`, `Services`, `PrivateNumbers`, `SmsSenderAgents`, `Outbox`, `Inbox`, `SentBox`, `PhoneBooks`, `Transactions`, `Logs`, `Settings`, `Domains`.
*   **Database System**: Microsoft SQL Server.

## 4. Component Interactions & Data Flow

1.  **User via UI**:
    *   User interacts with ASP.NET WebForms UI (`Arad.SMS.Gateway.Web`).
    *   UI components call the `Facade` layer (`Arad.SMS.Gateway.Facade`) for operations.
    *   Facade uses `Business` layer, which in turn uses `DataAccessLayer` to interact with the SQL Server DB.

2.  **External Application via API**:
    *   External client calls `Arad.SMS.Gateway.WebApi`.
    *   API controllers process requests, often using the `Facade` or `Business` layer.
    *   For long-running tasks (like sending SMS), requests might be queued.

3.  **Queue Processing**:
    *   `ApiProcessRequest` background worker picks up queued tasks (e.g., from API).
    *   It processes these tasks, interacting with `DataLayer` and potentially `SMSProviderWorker`s.

4.  **SMS Sending**:
    *   A request to send SMS (from UI, API, or scheduled task) is processed.
    *   Routing logic (likely in `Business` or `Facade`) selects an `SmsSenderAgent`.
    *   The corresponding `SMSProviderWorker` (e.g., `AradSmsSender`) sends the SMS via the provider's API.
    *   `SaveSentSms` worker archives the sent message details.

5.  **Delivery Reports**:
    *   `GetSmsDelivery` worker periodically polls providers for delivery statuses.
    *   `SaveSmsDelivery` worker saves these statuses to the database.

6.  **Incoming SMS**:
    *   Providers push incoming SMS to an endpoint (likely `RecieveSms.aspx` or a dedicated API endpoint).
    *   `MessageParser` worker processes these messages based on rules.

7.  **General Operations**:
    *   All modules utilize `GeneralLibrary` for common functions like logging, configuration, and encryption.
    *   `SqlLibrary` offers specialized database utilities.

## 5. Implementation Details

*   **Language**: Primarily C#.
*   **Framework**: .NET Framework (various versions, up to 4.6.1 noted in Web project).
*   **Database**: Microsoft SQL Server.
*   **Web Technologies**: ASP.NET WebForms, ASP.NET Web API, OWIN, HTML, CSS, JavaScript.
*   **Windows Services**: Used for background workers and API self-hosting, often managed with Topshelf.
*   **Concurrency**: Multi-threading is used in background workers and SMS senders.
*   **Data Access**: ADO.NET with stored procedures.
*   **Modularity**: The project is divided into multiple class libraries and executables, promoting separation of concerns.
*   **Configuration**: XML-based configuration files (`Configortion.config`, `app.config`, `web.config`).
*   **Error Handling**: Custom exception handling attributes and logging mechanisms are in place.
*   **Security**: Includes IP authentication, custom authentication attributes for APIs, and data encryption utilities.

## 6. Deployment

*   The system consists of a web application (ASP.NET WebForms) to be hosted on IIS.
*   Multiple Windows Services (API, Background Workers, SMS Provider Workers) need to be installed and run on a server.
*   A SQL Server database needs to be set up using the provided SQL scripts.

## 7. Potential Areas for Modernization (Reviewer Notes)

*   The UI layer uses ASP.NET WebForms, which is an older technology. Migration to a more modern framework like ASP.NET Core MVC/Razor Pages or a SPA framework (React, Angular, Vue) could be considered for future development.
*   Data access is primarily through ADO.NET and stored procedures. While performant, an ORM like Entity Framework Core could simplify development and maintenance for some parts of the application.
*   Consideration for containerization (e.g., Docker) could simplify deployment and scaling.
*   The project structure, while modular, has many small projects. Some consolidation might be beneficial.
*   The use of "TerafficRelay" (misspelled) and "Configortion.config" (misspelled) should be corrected for consistency.
