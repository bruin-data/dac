import { bruinDarkTemplate, bruinTemplate, type DashboardTemplate } from '../src/lib';

// Mirrors Bruin Desktop's src/renderer/src/features/dashboards/theme.ts so dashboards look the same in both.
// When Desktop's tokens change, update this copy too.
const light: Record<string, string> = {
    background: '#fafafa',
    surface: '#ffffff',
    'surface-hover': '#f4f4f5',
    border: '#e6e6e8',
    'text-primary': '#141416',
    'text-secondary': '#56565c',
    'text-muted': '#6b6b72',
    accent: '#cc3348',
    'accent-hover': '#b02a3d',
    'accent-subtle': 'rgba(204, 51, 72, 0.07)',
    success: '#047857',
    warning: '#a35a00',
    error: '#b42318',
    'chart-1': '#2a78d6',
    'chart-2': '#eb6834',
    'chart-3': '#1baf7a',
    'chart-4': '#eda100',
    'chart-5': '#e87ba4',
    'chart-6': '#008300',
    'chart-7': '#4a3aa7',
    'chart-8': '#e34948',
};

const dark: Record<string, string> = {
    background: '#0a0a0b',
    surface: '#141416',
    'surface-hover': '#1c1c1f',
    border: '#26262a',
    'text-primary': '#ededee',
    'text-secondary': '#a0a0a6',
    'text-muted': '#85858c',
    accent: '#ff6b7d',
    'accent-hover': '#ff8796',
    'accent-subtle': 'rgba(255, 107, 125, 0.10)',
    success: '#34d399',
    warning: '#f0b05a',
    error: '#f87171',
    'chart-1': '#3987e5',
    'chart-2': '#d95926',
    'chart-3': '#199e70',
    'chart-4': '#c98500',
    'chart-5': '#d55181',
    'chart-6': '#2f9e2f',
    'chart-7': '#9085e9',
    'chart-8': '#e66767',
};

export const lightTemplate: DashboardTemplate = { ...bruinTemplate, name: 'bruin-desktop', tokens: light };
export const darkTemplate: DashboardTemplate = { ...bruinDarkTemplate, name: 'bruin-desktop-dark', tokens: dark };
