import React, { useState } from 'react';
import '../App.css';
import type { Inventory, Host, Group } from '../types';
import {
    Server,
    Plus,
    Trash2,
    Monitor,
    Users
} from 'lucide-react';

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
    const [activeTab, setActiveTab] = useState<'hosts' | 'groups'>('hosts');
    const [showNewInv, setShowNewInv] = useState(false);

    const handleCreateInv = () => {
        if (newInvName) {
            onCreateInventory(newInvName);
            setNewInvName('');
            setShowNewInv(false);
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

    const selectedInv = inventories.find(i => i.id === selectedInventoryId);

    return (
        <div style={{ display: 'grid', gridTemplateColumns: '250px 1fr', gap: '2rem', height: '100%' }}>
            {/* LEFT SIDEBAR: INVENTORY LIST */}
            <div className="card" style={{ display: 'flex', flexDirection: 'column', height: 'fit-content' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
                    <h3 style={{ margin: 0, fontSize: '1.1rem' }}>Inventories</h3>
                    <button
                        className="action-btn-sm"
                        onClick={() => setShowNewInv(!showNewInv)}
                        title="New Inventory"
                    >
                        <Plus size={16} />
                    </button>
                </div>

                {showNewInv && (
                    <div style={{ marginBottom: '1rem', paddingBottom: '1rem', borderBottom: '1px solid #333' }}>
                        <input
                            placeholder="Name..."
                            value={newInvName}
                            onChange={e => setNewInvName(e.target.value)}
                            style={{ marginBottom: '0.5rem', width: '100%' }}
                            autoFocus
                        />
                        <button onClick={handleCreateInv} style={{ width: '100%', fontSize: '0.8rem' }}>Create</button>
                    </div>
                )}

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                    {inventories.map(inv => (
                        <div
                            key={inv.id}
                            onClick={() => onSelectInventory(inv.id)}
                            style={{
                                padding: '10px',
                                borderRadius: '6px',
                                cursor: 'pointer',
                                backgroundColor: selectedInventoryId === inv.id ? 'rgba(100, 108, 255, 0.15)' : 'transparent',
                                border: selectedInventoryId === inv.id ? '1px solid #646cff' : '1px solid transparent',
                                display: 'flex',
                                alignItems: 'center',
                                gap: '0.5rem',
                                transition: 'all 0.2s'
                            }}
                        >
                            <Server size={16} color={selectedInventoryId === inv.id ? '#646cff' : '#666'} />
                            <span style={{ fontWeight: selectedInventoryId === inv.id ? 500 : 400 }}>{inv.name}</span>
                        </div>
                    ))}
                    {inventories.length === 0 && <div style={{ color: '#666', fontStyle: 'italic', fontSize: '0.9rem' }}>No inventories yet.</div>}
                </div>
            </div>

            {/* MAIN CONTENT AREA */}
            <div>
                {!selectedInventoryId ? (
                    <div className="card" style={{ textAlign: 'center', padding: '3rem', color: '#666' }}>
                        <Server size={48} style={{ marginBottom: '1rem', opacity: 0.5 }} />
                        <h3>Select an Inventory</h3>
                        <p>Choose an inventory from the left commands to manage hosts and groups.</p>
                    </div>
                ) : (
                    <div>
                        {/* HEADER */}
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
                            <div>
                                <h2 style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                                    {selectedInv?.name}
                                    <span style={{ fontSize: '0.8rem', padding: '2px 8px', borderRadius: '12px', backgroundColor: '#333', color: '#aaa', fontWeight: 'normal' }}>
                                        ID: {selectedInv?.id}
                                    </span>
                                </h2>
                            </div>

                            {/* TABS */}
                            <div style={{ display: 'flex', gap: '0.5rem', backgroundColor: '#1a1a1a', padding: '4px', borderRadius: '8px' }}>
                                <button
                                    onClick={() => setActiveTab('hosts')}
                                    style={{
                                        backgroundColor: activeTab === 'hosts' ? '#333' : 'transparent',
                                        color: activeTab === 'hosts' ? 'white' : '#888',
                                        border: 'none',
                                        padding: '6px 16px',
                                        borderRadius: '6px',
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: '6px'
                                    }}
                                >
                                    <Monitor size={16} /> Hosts
                                </button>
                                <button
                                    onClick={() => setActiveTab('groups')}
                                    style={{
                                        backgroundColor: activeTab === 'groups' ? '#333' : 'transparent',
                                        color: activeTab === 'groups' ? 'white' : '#888',
                                        border: 'none',
                                        padding: '6px 16px',
                                        borderRadius: '6px',
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: '6px'
                                    }}
                                >
                                    <Users size={16} /> Groups
                                </button>
                            </div>
                        </div>

                        {/* CONTENT */}
                        {activeTab === 'hosts' && (
                            <div className="card">
                                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
                                    <h3>Managed Hosts</h3>
                                </div>

                                {/* ADD HOST FORM (Inline-ish) */}
                                <div style={{ backgroundColor: '#1a1a1a', padding: '1rem', borderRadius: '8px', marginBottom: '1rem', display: 'grid', gridTemplateColumns: 'minmax(150px, 200px) 1fr auto', gap: '1rem', alignItems: 'start' }}>
                                    <input
                                        placeholder="Hostname (e.g. web1)"
                                        value={newHostName}
                                        onChange={e => setNewHostName(e.target.value)}
                                        style={{ height: '36px' }}
                                    />
                                    <textarea
                                        placeholder='Vars (e.g. {"ansible_host": "1.2.3.4"})'
                                        value={newHostVars}
                                        onChange={e => setNewHostVars(e.target.value)}
                                        rows={1}
                                        style={{ height: '36px', resize: 'none', fontFamily: 'monospace' }}
                                    />
                                    <button onClick={handleCreateHost} style={{ height: '36px', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                        <Plus size={16} /> Add
                                    </button>
                                </div>

                                <table className="data-table">
                                    <thead>
                                        <tr>
                                            <th style={{ width: '40px' }}>STS</th>
                                            <th>Hostname</th>
                                            <th>Variables</th>
                                            <th style={{ width: '80px', textAlign: 'right' }}>Actions</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {hosts.map(h => (
                                            <tr key={h.id}>
                                                <td style={{ textAlign: 'center' }}>
                                                    <div style={{
                                                        width: '10px', height: '10px', borderRadius: '50%',
                                                        backgroundColor: h.enabled ? '#4ade80' : '#666',
                                                        margin: '0 auto'
                                                    }} title={h.enabled ? 'Enabled' : 'Disabled'} />
                                                </td>
                                                <td style={{ fontWeight: 500 }}>{h.name}</td>
                                                <td style={{ fontFamily: 'monospace', color: '#888', fontSize: '0.85rem' }}>
                                                    {JSON.stringify(h.variables).substring(0, 50)}
                                                    {JSON.stringify(h.variables).length > 50 && '...'}
                                                </td>
                                                <td style={{ textAlign: 'right' }}>
                                                    <button
                                                        className="action-btn-sm"
                                                        style={{ color: '#ef4444', backgroundColor: 'transparent', padding: '4px' }}
                                                        onClick={() => onDeleteHost(h.id)}
                                                        title="Delete Host"
                                                    >
                                                        <Trash2 size={16} />
                                                    </button>
                                                </td>
                                            </tr>
                                        ))}
                                        {hosts.length === 0 && (
                                            <tr><td colSpan={4} style={{ textAlign: 'center', padding: '2rem', color: '#666' }}>No hosts found in this inventory.</td></tr>
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        )}

                        {activeTab === 'groups' && (
                            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
                                {/* GROUP LIST */}
                                <div className="card">
                                    <h3>Groups</h3>
                                    <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem' }}>
                                        <input
                                            placeholder="New Group Name"
                                            value={newGroupName}
                                            onChange={e => setNewGroupName(e.target.value)}
                                            style={{ flex: 1 }}
                                        />
                                        <button onClick={handleCreateGroup}><Plus size={16} /></button>
                                    </div>
                                    <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                                        {groups.map(g => (
                                            <div
                                                key={g.id}
                                                onClick={() => onSelectGroup(g.id)}
                                                style={{
                                                    padding: '10px',
                                                    borderRadius: '6px',
                                                    cursor: 'pointer',
                                                    backgroundColor: selectedGroupId === g.id ? '#252525' : 'transparent',
                                                    border: selectedGroupId === g.id ? '1px solid #646cff' : '1px solid #333',
                                                    display: 'flex',
                                                    justifyContent: 'space-between',
                                                    alignItems: 'center'
                                                }}
                                            >
                                                <span>{g.name}</span>
                                                <Users size={14} color="#666" />
                                            </div>
                                        ))}
                                        {groups.length === 0 && <div style={{ color: '#666' }}>No groups yet.</div>}
                                    </div>
                                </div>

                                {/* GROUP DETAILS */}
                                {selectedGroupId ? (
                                    <div className="card" style={{ border: '1px solid #444' }}>
                                        <h4 style={{ marginTop: 0, color: '#646cff' }}>
                                            {groups.find(g => g.id === selectedGroupId)?.name} Members
                                        </h4>
                                        <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem' }}>
                                            <select
                                                value={hostIdToAdd || ''}
                                                onChange={e => setHostIdToAdd(parseInt(e.target.value))}
                                                style={{ flex: 1 }}
                                            >
                                                <option value="">Add host to group...</option>
                                                {hosts.map(h => <option key={h.id} value={h.id}>{h.name}</option>)}
                                            </select>
                                            <button onClick={() => hostIdToAdd && onAddHostToGroup(hostIdToAdd)}>Add</button>
                                        </div>

                                        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
                                            {groupHosts.map(h => (
                                                <div key={h.id} style={{ padding: '8px', backgroundColor: '#111', borderRadius: '4px', fontSize: '0.9rem' }}>
                                                    {h.name}
                                                </div>
                                            ))}
                                            {groupHosts.length === 0 && <div style={{ color: '#666' }}>No hosts in group.</div>}
                                        </div>
                                    </div>
                                ) : (
                                    <div className="card" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666' }}>
                                        Select a group to manage members
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
};

export default InventoriesPage;
