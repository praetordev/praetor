import React, { useState } from 'react';
import '../App.css';
import type { JobTemplate, Project, Inventory } from '../types';

interface TemplatesPageProps {
    templates: JobTemplate[];
    projects: Project[];
    inventories: Inventory[];
    onCreateTemplate: (name: string, projectId: number, inventoryId: number, playbook: string) => void;
    onUpdateTemplate: (id: number, name: string, projectId: number, inventoryId: number, playbook: string) => void;
}

const TemplatesPage: React.FC<TemplatesPageProps> = ({
    templates,
    projects,
    inventories,
    onCreateTemplate,
    onUpdateTemplate
}) => {
    const [id, setId] = useState<number | null>(null); // For Edit Mode
    const [name, setName] = useState('');
    const [projectId, setProjectId] = useState<number | null>(null);
    const [inventoryId, setInventoryId] = useState<number | null>(null);
    const [playbook, setPlaybook] = useState('');

    const handleSave = () => {
        if (!name || !projectId || !inventoryId || !playbook) {
            alert("Please fill in all fields: Name, Playbook, Project, and Inventory.");
            return;
        }

        if (name && projectId && inventoryId && playbook) {
            if (id) {
                onUpdateTemplate(id, name, projectId, inventoryId, playbook);
            } else {
                onCreateTemplate(name, projectId, inventoryId, playbook);
            }
            resetForm();
        }
    };

    const handleEdit = (t: JobTemplate) => {
        setId(t.id);
        setName(t.name);
        setProjectId(t.project_id || null);
        setInventoryId(t.inventory_id || null);
        setPlaybook(t.playbook);
    };

    const resetForm = () => {
        setId(null);
        setName('');
        setProjectId(null);
        setInventoryId(null);
        setPlaybook('');
    };

    return (
        <div>
            <div className="header-actions">
                <h2 className="page-title">Job Templates</h2>
            </div>

            <div className="card">
                <h3>{id ? `Edit Template #${id}` : 'Create New Template'}</h3>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem', maxWidth: '800px' }}>
                    <div className="form-group">
                        <label>Name</label>
                        <input
                            placeholder="Template Name"
                            value={name}
                            onChange={e => setName(e.target.value)}
                        />
                    </div>
                    <div className="form-group">
                        <label>Playbook</label>
                        <input
                            placeholder="e.g. site.yml"
                            value={playbook}
                            onChange={e => setPlaybook(e.target.value)}
                        />
                    </div>
                    <div className="form-group">
                        <label>Project</label>
                        <select
                            value={projectId || ''}
                            onChange={e => setProjectId(e.target.value ? parseInt(e.target.value) : null)}
                        >
                            <option value="">Select Project...</option>
                            {projects.map(p => (
                                <option key={p.id} value={p.id}>{p.name}</option>
                            ))}
                        </select>
                    </div>
                    <div className="form-group">
                        <label>Inventory</label>
                        <select
                            value={inventoryId || ''}
                            onChange={e => setInventoryId(e.target.value ? parseInt(e.target.value) : null)}
                        >
                            <option value="">Select Inventory...</option>
                            {inventories.map(i => (
                                <option key={i.id} value={i.id}>{i.name}</option>
                            ))}
                        </select>
                    </div>
                </div>
                <div style={{ marginTop: '1rem', display: 'flex', gap: '1rem' }}>
                    <button onClick={handleSave}>{id ? 'Update Template' : 'Create Template'}</button>
                    {id && <button onClick={resetForm} style={{ backgroundColor: '#666' }}>Cancel</button>}
                </div>
            </div>

            <div className="card">
                <table className="data-table">
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Name</th>
                            <th>Playbook</th>
                            <th>Project ID</th>
                            <th>Inventory ID</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {templates.map(t => (
                            <tr key={t.id}>
                                <td>{t.id}</td>
                                <td>{t.name}</td>
                                <td>{t.playbook}</td>
                                <td>{t.project_id}</td>
                                <td>{t.inventory_id}</td>
                                <td>
                                    <button
                                        className="action-btn-sm"
                                        onClick={() => handleEdit(t)}
                                        title="Edit Template"
                                    >
                                        ✏️ Edit
                                    </button>
                                </td>
                            </tr>
                        ))}
                        {templates.length === 0 && (
                            <tr><td colSpan={6} style={{ textAlign: 'center' }}>No templates found</td></tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
};

export default TemplatesPage;
