import React, { type ReactNode } from 'react';

interface LayoutProps {
    children: ReactNode;
    sidebar: ReactNode;
}

const Layout: React.FC<LayoutProps> = ({ children, sidebar }) => {
    return (
        <div className="app-layout">
            {sidebar}
            <main className="main-content">
                {children}
            </main>
        </div>
    );
};

export default Layout;
