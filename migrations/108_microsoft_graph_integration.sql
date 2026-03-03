-- Migration 108: Add microsoft_graph to tenant_integrations integration_type
-- Phase 1 of Epic 4: Microsoft Graph Connector (pf-a81eba)

ALTER TABLE tenant_integrations
    DROP CONSTRAINT valid_integration_type,
    ADD CONSTRAINT valid_integration_type CHECK (
        integration_type IN ('jira', 'google_workspace', 'slack', 'confluence', 'github', 'linear', 'microsoft_graph')
    );
