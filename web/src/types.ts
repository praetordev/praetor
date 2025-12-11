export interface Job {
    id: number;
    name: string;
    status: string;
    created_at: string;
    current_run_id?: string;
}

export interface JobEvent {
    seq: number;
    event_type: string;
    stdout_snippet: string;
    created_at: string;
    current_run_id?: string;
    task_name?: string;
}

export interface Project {
    id: number;
    name: string;
    scm_url: string;
    scm_type: string;
    modified_at?: string;
}

export interface JobTemplate {
    id: number;
    name: string;
    project_id?: number;
    inventory_id?: number;
    playbook: string;
    organization_id: number;
}

export interface Inventory {
    id: number;
    name: string;
    organization_id: number;
}

export interface Host {
    id: number;
    inventory_id: number;
    name: string;
    variables?: object;
    enabled: boolean;
}

export interface Group {
    id: number;
    inventory_id: number;
    name: string;
    variables?: object;
}
