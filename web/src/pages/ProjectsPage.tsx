import React, { useState } from 'react';
import '../App.css';
import type { Project } from '../types';

interface ProjectsPageProps {
    projects: Project[];
    onCreateProject: (name: string, url: string) => void;
    onSyncProject: (id: number) => Promise<boolean>;
}

const ProjectsPage: React.FC<ProjectsPageProps> = ({ projects, onCreateProject, onSyncProject }) => {
    const [name, setName] = useState('');
    const [url, setUrl] = useState('');
    const [syncingMap, setSyncingMap] = useState<Record<number, boolean>>({});

    const handleCreate = () => {
        if (name && url) {
            onCreateProject(name, url);
            setName('');
            setUrl('');
        }
    };

    const handleSync = async (id: number) => {
        setSyncingMap(prev => ({ ...prev, [id]: true }));
        await onSyncProject(id);
        setSyncingMap(prev => ({ ...prev, [id]: false }));
    };

    return (
        <div>
            <div className="header-actions">
                <h2 className="page-title">Projects</h2>
            </div>

            <div className="card">
                <h3>Add New Project</h3>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', maxWidth: '500px' }}>
                    <input
                        placeholder="Project Name (e.g. My Ansible Repo)"
                        value={name}
                        onChange={e => setName(e.target.value)}
                    />
                    <input
                        placeholder="Git URL (e.g. https://github.com/user/repo.git)"
                        value={url}
                        onChange={e => setUrl(e.target.value)}
                    />
                    <button onClick={handleCreate} style={{ width: 'fit-content' }}>Add Project</button>
                </div>
            </div>

            <div className="card">
                <table className="data-table">
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Name</th>
                            <th>SCM URL</th>
                            <th>Type</th>
                            <th>Last Synced</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {projects.map(proj => (
                            <tr key={proj.id}>
                                <td>{proj.id}</td>
                                <td>{proj.name}</td>
                                <td>{proj.scm_url}</td>
                                <td><span className="status-badge" style={{ backgroundColor: '#333' }}>git</span></td>
                                <td style={{ fontSize: '0.85rem', color: '#888' }}>
                                    {proj.modified_at ? new Date(proj.modified_at).toLocaleString() : '-'}
                                </td>
                                <td>
                                    <button
                                        className="action-btn-sm"
                                        onClick={() => handleSync(proj.id)}
                                        disabled={syncingMap[proj.id]}
                                        title="Sync Project (Check Connection)"
                                        style={{ opacity: syncingMap[proj.id] ? 0.7 : 1 }}
                                    >
                                        {syncingMap[proj.id] ? '⏳ Syncing...' : '🔄 Sync'}
                                    </button>
                                </td>
                            </tr>
                        ))}
                        {projects.length === 0 && (
                            <tr><td colSpan={4} style={{ textAlign: 'center' }}>No projects found</td></tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
};

export default ProjectsPage;
