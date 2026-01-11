import { ReactNode } from 'react';
import { Navbar } from './Navbar';

interface LayoutProps {
  children: ReactNode;
}

export function Layout({ children }: LayoutProps) {
  return (
    <div className="min-h-screen bg-gray-50 dark:bg-black">
      <Navbar />
      <main className="mt-16 p-6">
        <div className="max-w-[1800px] mx-auto">
          {children}
        </div>
      </main>
    </div>
  );
}

