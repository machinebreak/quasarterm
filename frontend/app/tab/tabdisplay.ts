// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

const AutoTabNameRegex = /^T\d+$/;

function isAutoTabName(name: string): boolean {
    if (name == null || name.trim() === "") {
        return true;
    }
    return AutoTabNameRegex.test(name.trim());
}

function getFolderBasename(path: string): string {
    if (path == null) {
        return null;
    }
    const parts = path.split(/[\\/]/).filter((part) => part !== "");
    if (parts.length === 0) {
        return path;
    }
    return parts[parts.length - 1];
}

// Auto-generated tab names (T1, T2, ...) are replaced by the project folder
// name when the tab is bound to a project directory.
function getTabDisplayName(name: string, folderName: string): string {
    if (isAutoTabName(name) && folderName != null && folderName !== "") {
        return folderName;
    }
    return name ?? "";
}

export { getFolderBasename, getTabDisplayName, isAutoTabName };
