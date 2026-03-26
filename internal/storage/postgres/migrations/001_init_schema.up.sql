-- Database schema for epbf-monitoring

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Plugins table: stores information about loaded plugins
CREATE TABLE IF NOT EXISTS plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    version VARCHAR(50) NOT NULL,
    description TEXT,
    author VARCHAR(255),
    git_url TEXT NOT NULL,
    git_commit VARCHAR(40),
    git_branch VARCHAR(255) DEFAULT 'main',
    ebpf_s3_key TEXT,
    wasm_s3_key TEXT,
    manifest JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'pending', -- pending, building, ready, error
    build_log TEXT,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_plugins_status ON plugins(status);
CREATE INDEX idx_plugins_name ON plugins(name);
CREATE INDEX idx_plugins_created_at ON plugins(created_at);

-- Metrics table: stores metric definitions from plugins
CREATE TABLE IF NOT EXISTS metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- counter, gauge, histogram, summary
    help TEXT,
    labels JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(plugin_id, name)
);

CREATE INDEX idx_metrics_plugin_id ON metrics(plugin_id);
CREATE INDEX idx_metrics_name ON metrics(name);
CREATE INDEX idx_metrics_type ON metrics(type);

-- Filters table: stores PromQL-like filter expressions
CREATE TABLE IF NOT EXISTS filters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id UUID REFERENCES plugins(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    expression TEXT NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(plugin_id, name)
);

CREATE INDEX idx_filters_plugin_id ON filters(plugin_id);
CREATE INDEX idx_filters_name ON filters(name);

-- Dashboards table: stores Grafana Scenes configurations
CREATE TABLE IF NOT EXISTS dashboards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    config JSONB NOT NULL,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_dashboards_name ON dashboards(name);

-- Plugin events table: audit log for plugin operations
CREATE TABLE IF NOT EXISTS plugin_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL, -- created, building, ready, error, deleted
    message TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_plugin_events_plugin_id ON plugin_events(plugin_id);
CREATE INDEX idx_plugin_events_event_type ON plugin_events(event_type);
CREATE INDEX idx_plugin_events_created_at ON plugin_events(created_at);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_plugins_updated_at BEFORE UPDATE ON plugins
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_dashboards_updated_at BEFORE UPDATE ON dashboards
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Insert default dashboard
INSERT INTO dashboards (name, description, config, is_default) VALUES (
    'Default Dashboard',
    'Default dashboard with common metrics',
    '{
        "version": 1,
        "panels": [],
        "layout": "grid"
    }'::jsonb,
    true
) ON CONFLICT DO NOTHING;
