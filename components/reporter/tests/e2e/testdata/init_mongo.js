// Reporter E2E Test: MongoDB seed data for plugin_crm
db = db.getSiblingDB('plugin_crm');

db.holders.drop();

db.holders.insertMany([
    {
        name: "Jo\u00e3o Silva",
        document: "12345678901",
        type: "individual",
        email: "joao@example.com",
        phone: "+5511999990001",
        status: "active",
        created_at: new Date("2025-01-15T10:00:00Z")
    },
    {
        name: "Maria Santos",
        document: "23456789012",
        type: "individual",
        email: "maria@example.com",
        phone: "+5511999990002",
        status: "active",
        created_at: new Date("2025-03-20T11:00:00Z")
    },
    {
        name: "Pedro Costa",
        document: "34567890123",
        type: "individual",
        email: "pedro@example.com",
        phone: "+5511999990003",
        status: "pending",
        created_at: new Date("2025-05-10T12:00:00Z")
    },
    {
        name: "Tech Solutions Ltda",
        document: "11222333000144",
        type: "company",
        email: "contact@techsolutions.com",
        phone: "+5511999990004",
        status: "active",
        created_at: new Date("2025-02-01T09:00:00Z")
    },
    {
        name: "Global Trading SA",
        document: "55666777000155",
        type: "company",
        email: "info@globaltrading.com",
        phone: "+5511999990005",
        status: "suspended",
        created_at: new Date("2025-07-15T14:00:00Z")
    }
]);

// Create indexes
db.holders.createIndex({ status: 1 });
db.holders.createIndex({ type: 1 });
db.holders.createIndex({ created_at: 1 });
