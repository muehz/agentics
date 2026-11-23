# Mermaid diagram templates: detailed reference

> This file holds Mermaid.js syntax templates, patterns, and examples for all 6 core diagram types.
> [SKILL.md](../SKILL.md) links here.

## Architecture diagram (C4 model)

**Use for**: System context, container views, component diagrams

**Pattern**:

```mermaid
graph TB
    subgraph "System Context"
        User[👤 User<br/>Person]
        System[🏢 Application<br/>Software System]
    end

    subgraph "External Systems"
        ExtAPI[🔌 External API<br/>Software System]
        Database[(💾 Database<br/>Container)]
    end

    User -->|Uses| System
    System -->|Calls| ExtAPI
    System -->|Reads/Writes| Database

    classDef person fill:#08427b,stroke:#052e56,color:#fff
    classDef system fill:#1168bd,stroke:#0b4884,color:#fff
    classDef external fill:#999,stroke:#666,color:#fff
    classDef database fill:#438dd5,stroke:#2d6ca6,color:#fff

    class User person
    class System system
    class ExtAPI external
    class Database database
```

**Template**:

```text
graph TB
    {ComponentID}[{Icon} {Name}<br/>{Type}]

    {ComponentA} -->|{Relationship}| {ComponentB}

    classDef {styleClass} fill:{color},stroke:{borderColor},color:{textColor}
    class {ComponentID} {styleClass}
```

**Component Types**:

- **Person**: 👤 icon, fill:#08427b
- **Software System**: 🏢 icon, fill:#1168bd
- **Container**: 📦 icon, fill:#438dd5
- **External System**: 🔌 icon, fill:#999
- **Database**: 💾 icon (use `[( ... )]` shape), fill:#438dd5

## Sequence diagram

**Use for**: API call flows, authentication flows, distributed system interactions

**Pattern**:

```mermaid
sequenceDiagram
    actor User
    participant Frontend
    participant API
    participant Auth
    participant Database

    User->>Frontend: Click "Login"
    Frontend->>API: POST /auth/login
    API->>Auth: Validate credentials
    Auth->>Database: Query user
    Database-->>Auth: User data
    Auth-->>API: Token
    API-->>Frontend: 200 OK + JWT
    Frontend->>Frontend: Store token
    Frontend-->>User: Redirect to dashboard

    Note over Frontend,API: All API calls include<br/>Authorization: Bearer {token}

    rect rgb(240, 240, 240)
        Note right of Database: Token expires after 24h
    end
```

**Template**:

```text
sequenceDiagram
    actor {ActorName}
    participant {ParticipantName}

    {From}->>+{To}: {Message}
    {To}-->>-{From}: {Response}

    Note over {Participant1},{Participant2}: {Note text}

    alt {Condition}
        {Action if true}
    else {AltCondition}
        {Action if false}
    end

    loop {LoopCondition}
        {Repeated actions}
    end

    par {ParallelAction1}
        {Action 1}
    and {ParallelAction2}
        {Action 2}
    end
```

**Arrow Types**:

- `->`: Solid line, no arrowhead
- `-->`: Dashed line, no arrowhead
- `->>`: Solid line with arrowhead (request)
- `-->>`: Dashed line with arrowhead (response)
- `->>+`: Solid line with arrowhead. Activates the receiver's lifeline
- `-->>-`: Dashed line with arrowhead. Deactivates the sender's lifeline

## Flowchart

**Use for**: Decision logic, algorithms, process flows, conditional branching

**Pattern**:

```mermaid
flowchart TD
    Start([Start]) --> Input[/Input: User request/]
    Input --> Validate{Validate<br/>request?}

    Validate -->|Valid| ProcessA[Process request]
    Validate -->|Invalid| Error[Return error]

    ProcessA --> CheckAuth{Authenticated?}
    CheckAuth -->|Yes| Authorize{Authorized?}
    CheckAuth -->|No| AuthError[401 Unauthorized]

    Authorize -->|Yes| Execute[Execute action]
    Authorize -->|No| AuthZError[403 Forbidden]

    Execute --> Success[200 OK]

    Error --> End([End])
    AuthError --> End
    AuthZError --> End
    Success --> End

    style Start fill:#90EE90
    style End fill:#FFB6C1
    style Error fill:#FFB6C1
    style AuthError fill:#FFB6C1
    style AuthZError fill:#FFB6C1
    style Success fill:#90EE90
```

**Template**:

```text
flowchart TD
    {ID}([{Start/End node}])
    {ID}[{Process node}]
    {ID}{"{Decision node}"}
    {ID}[/{Input/Output}/]
    {ID}[(Database)]
    {ID}(({Event}))

    {From} --> {To}
    {From} -->|{Label}| {To}

    style {ID} fill:{color},stroke:{borderColor}
```

**Node Shapes**:

- `([...])`: Rounded rectangle (start/end)
- `[...]`: Rectangle (process)
- `{...}`: Diamond (decision)
- `[/..../]`: Parallelogram (input/output)
- `[(....)]`: Cylinder (database)
- `((....))`: Circle (event)

**Flow Directions**:

- `flowchart TD`: Top to bottom
- `flowchart LR`: Left to right
- `flowchart BT`: Bottom to top
- `flowchart RL`: Right to left

## Entity-relationship diagram

**Use for**: Database schemas, data models, table relationships

**Pattern**:

```mermaid
erDiagram
    User ||--o{ Order : places
    User {
        uuid id PK
        string email UK
        string name
        datetime created_at
    }

    Order ||--|{ OrderItem : contains
    Order {
        uuid id PK
        uuid user_id FK
        decimal total
        enum status
        datetime created_at
    }

    Product ||--o{ OrderItem : "ordered in"
    Product {
        uuid id PK
        string name
        decimal price
        int inventory
    }

    OrderItem {
        uuid id PK
        uuid order_id FK
        uuid product_id FK
        int quantity
        decimal price
    }
```

**Template**:

```text
erDiagram
    {Entity1} {Relationship} {Entity2} : {Label}

    {Entity} {
        {type} {column_name} {constraints}
    }
```

**Relationship Types**:

- `||--||`: One to one
- `||--o{`: One to zero or more
- `}o--o{`: Zero or more to zero or more
- `||--|{`: One to one or more
- `}o--||`: Zero or more to one

**Constraint Notations**:

- `PK`: Primary Key
- `FK`: Foreign Key
- `UK`: Unique Key

Mermaid supports only these three keys. Put other constraints, such as Not Null, in an attribute comment. An example is
`string email UK "NOT NULL"`.

## Class diagram

**Use for**: OOP class structures, inheritance hierarchies, interfaces

**Pattern**:

```mermaid
classDiagram
    class IRepository {
        <<interface>>
        +GetById(id: string) Entity
        +List(filter: Filter) List~Entity~
        +Create(entity: Entity) Entity
        +Update(entity: Entity) bool
        +Delete(id: string) bool
    }

    class BaseRepository {
        <<abstract>>
        #connection: DbConnection
        #Execute(query: string) Result
        +GetById(id: string)* Entity
    }

    class UserRepository {
        -cache: ICache
        +GetById(id: string) User
        +GetByEmail(email: string) User
        -ValidateUser(user: User) bool
    }

    IRepository <|.. BaseRepository : implements
    BaseRepository <|-- UserRepository : extends
    UserRepository --> User : uses

    class User {
        -id: string
        -email: string
        -name: string
        +Validate() bool
        +ToJson() string
    }
```

**Template**:

```text
classDiagram
    class {ClassName} {
        <<{stereotype}>>
        {visibility}{member}: {type}
        {visibility}{method}({params}) {returnType}
    }

    {Class1} <|-- {Class2} : extends
    {Class1} <|.. {Class2} : implements
    {Class1} --> {Class2} : uses
    {Class1} --* {Class2} : composition
    {Class1} --o {Class2} : aggregation
```

**Visibility Modifiers**:

- `+`: public
- `-`: private
- `#`: protected
- `~`: package/internal

**Stereotypes**:

- `<<interface>>`
- `<<abstract>>`
- `<<enum>>`
- `<<service>>`

**Relationship Types**:

- `<|--`: Inheritance (extends)
- `<|..`: Realization (implements)
- `-->`: Association (uses)
- `--*`: Composition (owns)
- `--o`: Aggregation (has)

## Deployment diagram

**Use for**: Infrastructure topology, server architecture, deployment environments

**Pattern** (a flowchart with custom styling):

```mermaid
graph TB
    subgraph "Region: us-east-1"
        subgraph "VPC: Production"
            subgraph "Public Subnet"
                ALB[⚖️ Application<br/>Load Balancer]
            end

            subgraph "Private Subnet A"
                App1[🚀 App Server 1<br/>t3.medium]
                Cache1[(🔴 Redis<br/>Primary)]
            end

            subgraph "Private Subnet B"
                App2[🚀 App Server 2<br/>t3.medium]
                Cache2[(🔴 Redis<br/>Replica)]
            end

            subgraph "Database Subnet"
                DB1[(💾 PostgreSQL<br/>Primary)]
                DB2[(💾 PostgreSQL<br/>Standby)]
            end
        end
    end

    Internet[🌐 Internet] --> ALB
    ALB --> App1
    ALB --> App2
    App1 --> Cache1
    App2 --> Cache1
    Cache1 -.->|Replication| Cache2
    App1 --> DB1
    App2 --> DB1
    DB1 -.->|Standby| DB2

    classDef infrastructure fill:#1168bd,stroke:#0b4884,color:#fff
    classDef database fill:#438dd5,stroke:#2d6ca6,color:#fff
    classDef cache fill:#dc382d,stroke:#a62920,color:#fff

    class ALB,App1,App2 infrastructure
    class DB1,DB2 database
    class Cache1,Cache2 cache
```
