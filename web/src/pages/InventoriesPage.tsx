import React, { useState } from 'react';
import '../App.css';
import type { Inventory, Host, Group } from '../types';

interface InventoriesPageProps {
    inventories: Inventory[];
    hosts: Host[];
    groups: Group[];
    groupHosts: Host[];
    selectedInventoryId: number | null;
    selectedGroupId: number | null;
    onSelectInventory: (id: number | null) => void;
    onSelectGroup: (id: number | null) => void;
    onCreateInventory: (name: string) => void;
    onCreateHost: (name: string, vars: string) => void;
    onCreateGroup: (name: string) => void;
    onAddHostToGroup: (hostId: number) => void;
    onDeleteHost: (hostId: number) => void;
}

const InventoriesPage: React.FC<InventoriesPageProps> = ({
    inventories,
    hosts,
    groups,
    groupHosts,
    selectedInventoryId,
    selectedGroupId,
    onSelectInventory,
    onSelectGroup,
    onCreateInventory,
    onCreateHost,
    onCreateGroup,
    onAddHostToGroup,
    onDeleteHost
}) => {
    const [newInvName, setNewInvName] = useState('');
    const [newHostName, setNewHostName] = useState('');
    const [newHostVars, setNewHostVars] = useState('');
    const [newGroupName, setNewGroupName] = useState('');
    const [hostIdToAdd, setHostIdToAdd] = useState<number | null>(null);

    const handleCreateInv = () => {
        if (newInvName) {
            onCreateInventory(newInvName);
            setNewInvName('');
        }
    };

    const handleCreateHost = () => {
        if (newHostName) {
            onCreateHost(newHostName, newHostVars);
            setNewHostName('');
            setNewHostVars('');
        }
    };

    const handleCreateGroup = () => {
        if (newGroupName) {
            onCreateGroup(newGroupName);
            setNewGroupName('');
        }
    };

    return (
        <div>
            <div className="header-actions">
                <h2 className="page-title">Inventories</h2>
            </div>

            <div className="card" style={{ display: 'flex', gap: '1rem', alignItems: 'flex-end' }}>
                <div style={{ flex: 1 }}>
                    <label style={{ display: 'block', marginBottom: '5px', color: '#aaa' }}>Select Active Inventory</label>
                    <select
                        value={selectedInventoryId || ''}
                        onChange={e => onSelectInventory(e.target.value ? parseInt(e.target.value) : null)}
                        style={{ padding: '8px', width: '100%' }}
                    >
                        <option value="">-- Select Inventory --</option>
                        {inventories.map(i => (
                            <option key={i.id} value={i.id}>{i.name}</option>
                        ))}
                    </select>
                </div>
                <div style={{ flex: 1, display: 'flex', gap: '0.5rem' }}>
                    <input
                        placeholder="New Inventory Name"
                        value={newInvName}
                        onChange={e => setNewInvName(e.target.value)}
                    />
                    <button onClick={handleCreateInv}>Create</button>
                </div>
            </div>

            {selectedInventoryId && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2rem' }}>

                    {/* HOSTS COLUMN */}
                    <div>
                        <h3>Hosts</h3>
                        <div className="card">
                            <h4>Add Host</h4>
                            <div className="form-group">
                                <input
                                    placeholder="Hostname (e.g. web1)"
                                    value={newHostName}
                                    onChange={e => setNewHostName(e.target.value)}
                                    style={{ marginBottom: '0.5rem' }}
                                />
                                <textarea
                                    placeholder='Variables JSON (e.g. {"ansible_host": "1.2.3.4"})'
                                    value={newHostVars}
                                    onChange={e => setNewHostVars(e.target.value)}
                                    rows={3}
                                />
                                <button onClick={handleCreateHost} style={{ marginTop: '0.5rem', width: '100%' }}>Add Host</button>
                            </div>
                        </div>

                        <div className="card">
                            <table className="data-table">
                                <thead>
                                    <tr>
                                        <th>ID</th>
                                        <th>Name</th>
                                        <th>Enabled</th>
                                        <th>Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {hosts.map(h => (
                                        <tr key={h.id}>
                                            <td>{h.id}</td>
                                            <td>{h.name}</td>
                                            <td style={{ color: h.enabled ? '#4ade80' : '#666' }}>{h.enabled ? 'Active' : 'Disabled'}</td>
                                            <td>
                                                <button
                                                    className="action-btn-sm"
                                                    style={{ backgroundColor: '#ef4444', color: 'white' }}
                                                    onClick={() => onDeleteHost(h.id)}
                                                >
                                                    Delete
                                                </button>
                                            </td>
                                        </tr>
                                    ))}
                                    {hosts.length === 0 && <tr><td colSpan={3} style={{ textAlign: 'center' }}>No hosts</td></tr>}
                                </tbody>
                            </table>
                        </div>
                    </div>

                    {/* GROUPS COLUMN */}
                    <div>
                        <h3>Groups</h3>
                        <div className="card">
                            <h4>Add Group</h4>
                            <div style={{ display: 'flex', gap: '0.5rem' }}>
                                <input
                                    placeholder="Group Name"
                                    value={newGroupName}
                                    onChange={e => setNewGroupName(e.target.value)}
                                />
                                <button onClick={handleCreateGroup}>Add</button>
                            </div>
                        </div>

                        <div className="card">
                            <table className="data-table">
                                <thead>
                                    <tr>
                                        <th>ID</th>
                                        <th>Name</th>
                                        <th>Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {groups.map(g => (
                                        <tr key={g.id} style={{ backgroundColor: selectedGroupId === g.id ? 'rgba(100, 108, 255, 0.1)' : 'transparent' }}>
                                            <td>{g.id}</td>
                                            <td>{g.name}</td>
                                            <td>
                                                <button className="action-btn-sm" onClick={() => onSelectGroup(g.id)}>
                                                    {selectedGroupId === g.id ? 'Selected' : 'Manage'}
                                                </button>
                                            </td>
                                        </tr>
                                    ))}
                                    {groups.length === 0 && <tr><td colSpan={3} style={{ textAlign: 'center' }}>No groups</td></tr>}
                                </tbody>
                            </table>
                        </div>

                        {selectedGroupId && (
                            <div className="card" style={{ border: '1px solid #646cff' }}>
                                <h4 style={{ color: '#818cf8' }}>Group: {groups.find(g => g.id === selectedGroupId)?.name}</h4>
                                <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem' }}>
                                    <select
                                        value={hostIdToAdd || ''}
                                        onChange={e => setHostIdToAdd(parseInt(e.target.value))}
                                        style={{ flex: 1 }}
                                    >
                                        <option value="">Select Host...</option>
                                        {hosts.map(h => <option key={h.id} value={h.id}>{h.name}</option>)}
                                    </select>
                                    <button onClick={() => hostIdToAdd && onAddHostToGroup(hostIdToAdd)}>Add Member</button>
                                </div>
                                <h5>Current Members:</h5>
                                <ul style={{ paddingLeft: '1.2rem', color: '#ccc' }}>
                                    {groupHosts.map(h => <li key={h.id}>{h.name}</li>)}
                                    {groupHosts.length === 0 && <li>No members yet</li>}
                                </ul>
                            </div>
                        )}
                    </div>

                </div>
            )}
        </div>
    );
};

export default InventoriesPage;
