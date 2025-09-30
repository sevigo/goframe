// This script runs inside the Goja runtime.
// It has access to the 'ts' object from typescript.js

function getVisibility(node, parentNode = null) {
    // For constructors, default to public unless specified otherwise.
    if (node.kind === ts.SyntaxKind.Constructor) {
        if (node.modifiers) {
            for (const modifier of node.modifiers) {
                if (modifier.kind === ts.SyntaxKind.PrivateKeyword) return 'private';
                if (modifier.kind === ts.SyntaxKind.ProtectedKeyword) return 'protected';
            }
        }
        return 'public';
    }

    // FIX: Check parent for export keyword on grouped variables.
    if (parentNode && parentNode.kind === ts.SyntaxKind.VariableStatement) {
         if (parentNode.modifiers && parentNode.modifiers.some(m => m.kind === ts.SyntaxKind.ExportKeyword)) {
            return 'public';
        }
    }

    if (node.modifiers) {
        for (const modifier of node.modifiers) {
            if (modifier.kind === ts.SyntaxKind.PublicKeyword) return 'public';
            if (modifier.kind === ts.SyntaxKind.PrivateKeyword) return 'private';
            if (modifier.kind === ts.SyntaxKind.ProtectedKeyword) return 'protected';
        }
    }
    const isExported = node.modifiers ? 
        node.modifiers.some(m => m.kind === ts.SyntaxKind.ExportKeyword) : false;
    return isExported ? 'public' : 'private';
}

function getDocumentation(node, sourceFile) {
    const fullText = sourceFile.getFullText();
    const commentRanges = ts.getLeadingCommentRanges(fullText, node.pos) || [];
    if (commentRanges.length === 0) return '';
    
    const commentText = commentRanges.map(range => fullText.slice(range.pos, range.end)).join('\n');
    return commentText.replace(/^\/\*\*?/, '').replace(/\*\/$/, '').replace(/^\s*\*\s?/gm, '').trim();
}

function getSignature(node, sourceFile) {
    if (ts.isMethodSignature(node) || ts.isMethodDeclaration(node) || ts.isFunctionDeclaration(node) || ts.isConstructorDeclaration(node)) {
        const bodyStart = node.body ? node.body.getStart(sourceFile) : node.getEnd();
        return sourceFile.getFullText().substring(node.getStart(sourceFile), bodyStart).trim();
    }
    return node.getText(sourceFile);
}

function processNode(node, sourceFile, parentIdentifier = '', parentNode = null) {
    const chunks = [], definitions = [], symbols = [];

    if (node.kind === ts.SyntaxKind.SourceFile || node.kind === ts.SyntaxKind.EndOfFileToken) {
        ts.forEachChild(node, (child) => {
            const result = processNode(child, sourceFile, parentIdentifier, node);
            chunks.push(...result.chunks); definitions.push(...result.definitions); symbols.push(...result.symbols);
        });
        return { chunks, definitions, symbols };
    }

    // FIX: Get start of node *without* JSDoc to get correct line number.
    const startPos = node.getStart(sourceFile, false);
    const start = sourceFile.getLineAndCharacterOfPosition(startPos);
    const end = sourceFile.getLineAndCharacterOfPosition(node.getEnd(sourceFile));
    const lineStart = start.line + 1;
    const lineEnd = end.line + 1;

    const content = node.getFullText(sourceFile).trim();
    const visibility = getVisibility(node, parentNode);
    const isExported = visibility === 'public';
    const documentation = getDocumentation(node, sourceFile);
    const signature = getSignature(node, sourceFile);

    let chunkType = '', defType = '', identifier = '', structureType = '', nodeName = node.name ? node.name.getText(sourceFile) : '(anonymous)';

    switch (node.kind) {
        case ts.SyntaxKind.FunctionDeclaration:
            chunkType = 'function'; defType = 'function'; identifier = nodeName = node.name.getText(sourceFile);
            break;
        case ts.SyntaxKind.ClassDeclaration:
            chunkType = 'type_declaration'; defType = 'class'; structureType = 'class'; identifier = nodeName = node.name.getText(sourceFile);
            break;
        case ts.SyntaxKind.InterfaceDeclaration:
            chunkType = 'type_declaration'; defType = 'interface'; structureType = 'interface'; identifier = nodeName = node.name.getText(sourceFile);
            break;
        case ts.SyntaxKind.EnumDeclaration:
            chunkType = 'type_declaration'; defType = 'enum'; structureType = 'enum'; identifier = nodeName = node.name.getText(sourceFile);
            break;
        case ts.SyntaxKind.TypeAliasDeclaration:
            chunkType = 'type_declaration'; defType = 'type'; structureType = 'alias'; identifier = nodeName = node.name.getText(sourceFile);
            break;
        case ts.SyntaxKind.VariableStatement:
            for (const decl of node.declarationList.declarations) {
                // Pass the parent VariableStatement down to correctly determine visibility.
                const result = processNode(decl, sourceFile, parentIdentifier, node);
                chunks.push(...result.chunks); definitions.push(...result.definitions); symbols.push(...result.symbols);
            }
            return { chunks, definitions, symbols };
        case ts.SyntaxKind.VariableDeclaration:
            chunkType = (node.parent.flags & ts.NodeFlags.Const) ? 'constant' : 'variable';
            defType = chunkType; identifier = nodeName = node.name.getText(sourceFile);
            break;
        case ts.SyntaxKind.MethodDeclaration:
        case ts.SyntaxKind.MethodSignature: // FIX: Handle methods in interfaces
            chunkType = 'function'; defType = 'method'; nodeName = node.name.getText(sourceFile); identifier = `${parentIdentifier}.${nodeName}`;
            break;
        case ts.SyntaxKind.PropertyDeclaration:
        case ts.SyntaxKind.PropertySignature: // FIX: Handle properties in interfaces
            chunkType = 'variable'; defType = 'property'; nodeName = node.name.getText(sourceFile); identifier = `${parentIdentifier}.${nodeName}`;
            break;
        case ts.SyntaxKind.Constructor:
            chunkType = 'function'; defType = 'constructor'; nodeName = 'constructor'; identifier = `${parentIdentifier}.constructor`;
            break;
    }

    if (chunkType) {
        chunks.push({
            content, lineStart, lineEnd, type: chunkType, identifier,
            annotations: {
                type: defType, name: nodeName, visibility, has_doc: (documentation.length > 0).toString(),
                ...(structureType && { structure_type: structureType }),
                is_method: (ts.isMethodDeclaration(node) || ts.isPropertyDeclaration(node) || ts.isConstructorDeclaration(node) || ts.isMethodSignature(node) || ts.isPropertySignature(node)).toString(),
                signature
            }, parentContext: parentIdentifier,
        });
        definitions.push({ type: defType, name: nodeName, lineStart, lineEnd, visibility, signature, documentation });
        if (parentIdentifier === '') symbols.push({ name: nodeName, type: defType, lineStart, lineEnd, isExported });
    }

    if (ts.isClassDeclaration(node) || ts.isInterfaceDeclaration(node)) {
        const newParentIdentifier = node.name.getText(sourceFile);
        (node.members || []).forEach(child => {
            const result = processNode(child, sourceFile, newParentIdentifier, node);
            chunks.push(...result.chunks); definitions.push(...result.definitions);
        });
    }
    return { chunks, definitions, symbols };
}

function createParser(funcName, sourceCode, sourcePath) {
    try {
        if (!sourceCode || sourceCode.trim() === '') {
            if (funcName === 'extractChunks') return JSON.stringify([]);
            if (funcName === 'extractMetadata') return JSON.stringify({ language: "typescript", imports: [], definitions: [], symbols: [], properties: {} });
        }
        const sourceFile = ts.createSourceFile(sourcePath, sourceCode, ts.ScriptTarget.Latest, true);
        const diagnostics = sourceFile.statements.length > 0 ? sourceFile.parseDiagnostics : [];
        if (diagnostics.length > 0) {
            const message = diagnostics.map(d => typeof d.messageText === 'string' ? d.messageText : d.messageText.messageText).join('\n');
            throw new Error(message);
        }

        if (funcName === 'extractChunks') {
            const { chunks } = processNode(sourceFile, sourceFile);
            return JSON.stringify(chunks, null, 2);
        }

        if (funcName === 'extractMetadata') {
            let imports = [], properties = { total_functions: 0, total_classes: 0, total_methods: 0 };
            
            // FIX: Use a recursive walker to find all nodes for accurate counts.
            function nodeWalker(node) {
                if (ts.isImportDeclaration(node)) imports.push(node.moduleSpecifier.getText(sourceFile).replace(/['"]/g, ''));
                if (ts.isFunctionDeclaration(node)) properties.total_functions++;
                if (ts.isClassDeclaration(node)) properties.total_classes++;
                if (ts.isMethodDeclaration(node)) properties.total_methods++;
                ts.forEachChild(node, nodeWalker);
            }
            nodeWalker(sourceFile);
            
            const { definitions, symbols } = processNode(sourceFile, sourceFile);
            return JSON.stringify({
                language: "typescript", imports, definitions, symbols,
                properties: {
                    total_functions: String(properties.total_functions),
                    total_classes: String(properties.total_classes),
                    total_methods: String(properties.total_methods),
                }
            }, null, 2);
        }
    } catch (e) {
        return JSON.stringify({ error: e.message });
    }
}

function extractChunks(sourceCode, sourcePath) { return createParser('extractChunks', sourceCode, sourcePath); }
function extractMetadata(sourceCode, sourcePath) { return createParser('extractMetadata', sourceCode, sourcePath); }