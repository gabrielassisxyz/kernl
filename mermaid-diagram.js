flowchart TD
    %% Estilos Visuais
    classDef artifact fill:#fdf6e3,stroke:#b58900,stroke-width:2px,color:#333;
    classDef skill fill:#268bd2,stroke:#002b36,stroke-width:1px,color:#fff;
    classDef beads fill:#859900,stroke:#002b36,stroke-width:2px,color:#fff;
    classDef foolery fill:#dc322f,stroke:#002b36,stroke-width:3px,color:#fff;
    classDef input fill:#eee,stroke:#333,stroke-width:2px,stroke-dasharray: 5 5,color:#333;
    classDef promotion stroke:#cb4b16,stroke-width:3px,color:#cb4b16;
    classDef userstart fill:#6c71c4,stroke:#002b36,stroke-width:2px,color:#fff;
    classDef loop fill:#d33682,stroke:#002b36,stroke-width:2px,color:#fff;
    classDef exit fill:#2aa198,stroke:#002b36,stroke-width:2px,color:#fff;

    %% Ponto de Entrada Principal (O Humano no Controle)
    START((O que vamos fazer hoje?)):::userstart

    START -->|Não sei direito...| I1
    START -->|Novo Produto| I2
    START -->|Novo Projeto Simples| I3
    START -->|Novo Projeto Importante| I4
    START -->|Adicionar Feature/Bug| I5

    %% 1. Fluxo de Ideação (Opcional)
    subgraph W1 [1. Ideias 100% Soltas]
        direction TB
        I1([Input: Ideia Vaga]):::input
        S1_1[ce-ideate]:::skill
        A1_1[/IDEATION_LOG.md/]:::artifact
        S1_2[superpowers/brainstorming]:::skill
        A1_2[/CONCEPT.md/]:::artifact

        I1 --> S1_1 --> A1_1 --> S1_2 --> A1_2
    end

    %% Caminhos de Promoção da Ideia Solta
    A1_2 ===|PROMOVER PARA PRODUTO| S2_1:::promotion
    A1_2 ===|PROMOVER PARA SIMPLES| S3_1:::promotion
    A1_2 ===|PROMOVER PARA IMPORTANTE| S4_1:::promotion
    A1_2 -. "Continuar Simples" .-> YEGGE_PLAN

    %% 2. Fluxo Comercial
    subgraph W2 [2. Ideias Comerciais]
        direction TB
        I2([Input: Ideia de Produto]):::input
        S2_1[gstack/office-hours]:::skill
        S2_2[gstack/plan-ceo-review...]:::skill
        S2_3[adversarial-spec]:::skill
        S2_4[automazeio/ccpm]:::skill
        A2_4[/EPICS.md/]:::artifact

        I2 --> S2_1 --> S2_2 --> S2_3 --> S2_4 --> A2_4
    end

    %% 3. Fluxo Simples / YOLO
    subgraph W3 [3. Projetos Simples]
        direction TB
        I3([Input: Ideia YOLO]):::input
        S3_1[gsd-new-project]:::skill
        S3_2[gsd-plan-phase]:::skill
        S3_3[planning-with-files]:::skill
        A3_3[/task_plan.md/]:::artifact

        I3 --> S3_1 --> S3_2 --> S3_3 --> A3_3
    end

    %% 4. Fluxo Importante
    subgraph W4 [4. Projetos Importantes]
        direction TB
        I4([Input: Ideia Importante]):::input
        S4_1[compound-engineering/ce-strategy]:::skill
        S4_2[gstack/plan-eng-review]:::skill
        S4_3[superpowers/writing-plans]:::skill
        A4_3[/TDD_PLAN.md/]:::artifact

        I4 --> S4_1 --> S4_2 --> S4_3 --> A4_3
    end

    %% 5. Fluxo de Manutenção (Projetos Existentes)
    subgraph W5 [5. Tarefas em Projetos Existentes]
        direction TB
        I5([Input: Task Simples/Complexa]):::input
        S5_1[Ler Memory Bank & AGENTS.md]:::skill
        A5_1[/Task Spec/]:::artifact
        
        I5 --> S5_1 --> A5_1
    end

    %% ==========================================
    %% FUNIL DE CONVERGÊNCIA E EXECUÇÃO
    %% ==========================================

    %% Todos os planos desembocam no Yegge Loop de Planejamento
    A2_4 --> YEGGE_PLAN
    A3_3 --> YEGGE_PLAN
    A4_3 --> YEGGE_PLAN
    A5_1 --> YEGGE_PLAN

    YEGGE_PLAN((Yegge Loop<br/>Collaborative Plan 5x)):::loop

    %% Handoff para o Gerador de Beads
    BG[Beads Generator / swarm-plan]:::skill
    YEGGE_PLAN --> BG

    %% Yegge Loop de Refinamento dos Beads
    YEGGE_BEADS((Yegge Loop<br/>Beads Refinement 5x)):::loop
    BG --> YEGGE_BEADS

    %% Execução com Foolery
    BEADS[".beads/ (Diretório)"]:::beads
    YEGGE_BEADS --> BEADS

    FOOLERY((Foolery Executor)):::foolery
    BEADS --> FOOLERY

    %% ==========================================
    %% ENCERRAMENTO E APRENDIZADO
    %% ==========================================
    subgraph EXIT [6. Session Completion & Memory]
        direction TB
        E1{Testes e git push OK?}:::exit
        M_BANK[/Memory Bank<br/>activeContext.md & progress.md/]:::artifact
        CE_COMP[Skill: /ce-compound]:::skill
        AGENTS_UPD[/AGENTS.md<br/>Atualiza Known Hurdles/]:::artifact

        E1 -->|Sim| M_BANK
        E1 -->|Sim| CE_COMP
        CE_COMP --> AGENTS_UPD
    end

    FOOLERY -->|Sessão Concluída| E1
