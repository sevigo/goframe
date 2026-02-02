const fs = require('fs');
const path = require('path');

const EXCLUDE_DIRS = ['.git', 'node_modules', 'vendor', '.github', 'testdata', 'brain', '.agent'];
const INCLUDE_EXTS = ['.go', '.md'];
const OUTPUT_FILE = 'llm_context.txt';

/**
 * Recursively gets all files in a directory that match the extensions
 * @param {string} dirPath 
 * @param {string[]} arrayOfFiles 
 * @returns {string[]}
 */
function getAllFiles(dirPath, arrayOfFiles = []) {
    const files = fs.readdirSync(dirPath);

    files.forEach(function (file) {
        const fullPath = path.join(dirPath, file);
        if (fs.statSync(fullPath).isDirectory()) {
            if (!EXCLUDE_DIRS.includes(file)) {
                arrayOfFiles = getAllFiles(fullPath, arrayOfFiles);
            }
        } else {
            const ext = path.extname(file).toLowerCase();
            if (INCLUDE_EXTS.includes(ext) && file !== OUTPUT_FILE) {
                arrayOfFiles.push(fullPath);
            }
        }
    });

    return arrayOfFiles;
}

function main() {
    console.log('Starting concatenation...');
    const rootDir = process.cwd();
    const files = getAllFiles(rootDir);

    let combinedContent = '';

    files.forEach((file) => {
        const relativePath = path.relative(rootDir, file);
        const content = fs.readFileSync(file, 'utf8');

        combinedContent += `\n--- FILE: ${relativePath} ---\n`;
        combinedContent += content;
        combinedContent += `\n--- END FILE: ${relativePath} ---\n`;
    });

    fs.writeFileSync(OUTPUT_FILE, combinedContent, 'utf8');
    console.log(`Success! Combined ${files.length} files into ${OUTPUT_FILE}`);
}

main();
