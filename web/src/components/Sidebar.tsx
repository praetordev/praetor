import React from 'react';
import '../App.css'; // We'll update App.css next for component styles

interface SidebarProps {
    activeTab: string;
    onTabChange: (tab: string) => void;
}

const Sidebar: React.FC<SidebarProps> = ({ activeTab, onTabChange }) => {
    const menuItems = [
        { id: 'dashboard', label: 'Dashboard', icon: '📊' },
        { id: 'jobs', label: 'Jobs', icon: '🚀' },
        { id: 'templates', label: 'Templates', icon: '📄' },
        { id: 'projects', label: 'Projects', icon: '📦' },
        { id: 'inventories', label: 'Inventories', icon: '🌐' },
    ];

    return (
        <aside className="sidebar">
            <div className="sidebar-header">
                <div className="logo-text">PRAETOR</div>
            </div>
            <nav className="sidebar-nav">
                {menuItems.map((item) => (
                    <button
                        key={item.id}
                        className={`nav-item ${activeTab === item.id ? 'active' : ''}`}
                        onClick={() => onTabChange(item.id)}
                    >
                        <span className="nav-icon">{item.icon}</span>
                        <span className="nav-label">{item.label}</span>
                    </button>
                ))}
            </nav>
            <div className="sidebar-footer">
                <div className="version-info">v0.1.0-alpha</div>
            </div>
        </aside>
    );
};

export default Sidebar;
