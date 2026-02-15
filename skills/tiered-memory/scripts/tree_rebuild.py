#!/usr/bin/env python3
"""
LLM-powered tree rebuild for tiered memory.
Analyzes memory patterns and restructures tree for optimal navigation.
"""

import json
import os
import sys
import urllib.request
import re
from collections import Counter

def llm_rebuild_tree(tree_nodes, warm_facts, llm_endpoint, api_key=None):
    """
    Use LLM to analyze patterns and suggest tree restructuring.
    
    Args:
        tree_nodes: Current tree structure (dict)
        warm_facts: Recent facts for pattern analysis (list)
        llm_endpoint: LLM API endpoint
        api_key: Optional API key
    
    Returns:
        Updated tree nodes (dict)
    """
    # Analyze current patterns
    category_counts = Counter()
    topic_words = Counter()
    
    for fact in warm_facts:
        category = fact.get('category', '')
        category_counts[category] += 1
        
        # Extract topic words from fact text
        text = fact.get('text', '').lower()
        words = re.findall(r'\b[a-z]{4,}\b', text)
        topic_words.update(words)
    
    # Find frequently co-occurring categories (potential merges)
    category_list = list(category_counts.keys())
    
    # Build prompt for LLM
    current_tree = []
    for path, node in tree_nodes.items():
        if path == 'root' or not isinstance(node, dict):
            continue
        current_tree.append({
            'path': path,
            'desc': node.get('desc', ''),
            'warm_count': node.get('warm_count', 0),
            'cold_count': node.get('cold_count', 0)
        })
    
    # Get top patterns
    top_categories = category_counts.most_common(20)
    top_topics = topic_words.most_common(30)
    
    prompt = f"""You are maintaining a memory tree index for an AI agent.

Current tree structure ({len(current_tree)} nodes):
{json.dumps(current_tree, indent=2)}

Recent activity patterns:
Top categories: {top_categories}
Top topics: {top_topics}

Tasks:
1. Identify categories that should be merged (too similar)
2. Suggest new categories for topics appearing 3+ times but uncategorized
3. Remove stale categories with zero activity in 60+ days
4. Update descriptions to reflect current state
5. Ensure tree depth stays ≤ 4 levels
6. Ensure total nodes stay ≤ 50

Output JSON format:
{{
  "merge": [
    {{"from": "path/to/old", "to": "path/to/new", "reason": "..."}},
    ...
  ],
  "add": [
    {{"path": "new/category", "desc": "Description", "reason": "..."}},
    ...
  ],
  "remove": [
    {{"path": "stale/category", "reason": "..."}},
    ...
  ],
  "update_desc": [
    {{"path": "existing/category", "desc": "New description", "reason": "..."}},
    ...
  ]
}}

Only suggest changes that improve organization. If structure is good, return empty arrays.
"""

    # Call LLM
    try:
        headers = {
            'Content-Type': 'application/json',
        }
        if api_key:
            headers['Authorization'] = f'Bearer {api_key}'
        
        data = {
            'messages': [
                {'role': 'user', 'content': prompt}
            ],
            'max_tokens': 2000,
            'temperature': 0.3
        }
        
        req = urllib.request.Request(
            llm_endpoint,
            data=json.dumps(data).encode(),
            headers=headers
        )
        
        with urllib.request.urlopen(req, timeout=30) as response:
            result = json.loads(response.read())
            
            # Try to extract response text
            if 'choices' in result:
                content = result['choices'][0]['message']['content']
            elif 'text' in result:
                content = result['text']
            else:
                content = str(result)
            
            # Parse JSON from response
            suggestions = json.loads(content.strip())
            return suggestions
            
    except Exception as e:
        print(f"LLM rebuild error: {e}", file=sys.stderr)
        return None

def apply_tree_changes(tree_nodes, changes, warm_facts):
    """
    Apply suggested changes to tree structure.
    
    Args:
        tree_nodes: Current tree nodes
        changes: Suggested changes from LLM
        warm_facts: Facts to update categories
    
    Returns:
        Updated tree nodes + category mapping
    """
    updated_tree = dict(tree_nodes)
    category_mapping = {}  # old -> new mapping for fact updates
    
    # 1. Apply merges
    for merge in changes.get('merge', []):
        from_path = merge['from']
        to_path = merge['to']
        
        if from_path in updated_tree and to_path in updated_tree:
            # Merge counts
            updated_tree[to_path]['warm_count'] += updated_tree[from_path].get('warm_count', 0)
            updated_tree[to_path]['cold_count'] += updated_tree[from_path].get('cold_count', 0)
            
            # Remove from parent
            parent = from_path.rsplit('/', 1)[0] if '/' in from_path else 'root'
            if parent in updated_tree:
                children = updated_tree[parent].get('children', [])
                if from_path in children:
                    children.remove(from_path)
            
            # Delete old node
            del updated_tree[from_path]
            
            # Track mapping
            category_mapping[from_path] = to_path
            
            print(f"Merged: {from_path} → {to_path}", file=sys.stderr)
    
    # 2. Add new nodes
    for add in changes.get('add', []):
        path = add['path']
        desc = add['desc']
        
        if path not in updated_tree and len(updated_tree) < 50:
            # Ensure parent exists
            if '/' in path:
                parent_path = path.rsplit('/', 1)[0]
                if parent_path not in updated_tree:
                    # Auto-create parent
                    parent_desc = parent_path.split('/')[-1].replace('_', ' ').title()
                    updated_tree[parent_path] = {
                        'desc': parent_desc,
                        'warm_count': 0,
                        'cold_count': 0,
                        'last_access': 0,
                        'children': []
                    }
            
            parent = path.rsplit('/', 1)[0] if '/' in path else 'root'
            
            updated_tree[path] = {
                'desc': desc[:100],
                'warm_count': 0,
                'cold_count': 0,
                'last_access': 0,
                'children': []
            }
            
            if parent in updated_tree:
                updated_tree[parent].setdefault('children', []).append(path)
            
            print(f"Added: {path} - {desc}", file=sys.stderr)
    
    # 3. Remove stale nodes
    for remove in changes.get('remove', []):
        path = remove['path']
        
        if path in updated_tree and path != 'root':
            node = updated_tree[path]
            # Only remove if truly empty
            if node.get('warm_count', 0) == 0 and node.get('cold_count', 0) == 0:
                parent = path.rsplit('/', 1)[0] if '/' in path else 'root'
                if parent in updated_tree:
                    children = updated_tree[parent].get('children', [])
                    if path in children:
                        children.remove(path)
                
                del updated_tree[path]
                print(f"Removed: {path}", file=sys.stderr)
    
    # 4. Update descriptions
    for update in changes.get('update_desc', []):
        path = update['path']
        desc = update['desc']
        
        if path in updated_tree:
            updated_tree[path]['desc'] = desc[:100]
            print(f"Updated desc: {path}", file=sys.stderr)
    
    return updated_tree, category_mapping

def main():
    import argparse
    parser = argparse.ArgumentParser(description='LLM-powered tree rebuild')
    parser.add_argument('--tree-file', required=True, help='Path to tree JSON')
    parser.add_argument('--warm-file', required=True, help='Path to warm memory JSON')
    parser.add_argument('--llm-endpoint', required=True, help='LLM API endpoint')
    parser.add_argument('--api-key', help='API key for LLM')
    parser.add_argument('--dry-run', action='store_true', help='Show changes without applying')
    
    args = parser.parse_args()
    
    # Load data
    with open(args.tree_file) as f:
        tree_nodes = json.load(f)
    
    with open(args.warm_file) as f:
        warm_facts = json.load(f)
    
    # Get suggestions from LLM
    print(f"Analyzing {len(warm_facts)} facts across {len(tree_nodes)} nodes...", file=sys.stderr)
    changes = llm_rebuild_tree(tree_nodes, warm_facts, args.llm_endpoint, args.api_key)
    
    if not changes:
        print("LLM rebuild failed", file=sys.stderr)
        sys.exit(1)
    
    # Show changes
    print("\nSuggested changes:", file=sys.stderr)
    print(json.dumps(changes, indent=2))
    
    if args.dry_run:
        print("\n[DRY RUN] Not applying changes", file=sys.stderr)
        sys.exit(0)
    
    # Apply changes
    updated_tree, category_mapping = apply_tree_changes(tree_nodes, changes, warm_facts)
    
    # Update warm facts with new categories
    if category_mapping:
        for fact in warm_facts:
            old_cat = fact.get('category')
            if old_cat in category_mapping:
                fact['category'] = category_mapping[old_cat]
        
        # Save updated facts
        with open(args.warm_file, 'w') as f:
            json.dump(warm_facts, f, indent=2)
        
        print(f"\nUpdated {len(category_mapping)} fact categories", file=sys.stderr)
    
    # Save updated tree
    with open(args.tree_file, 'w') as f:
        json.dump(updated_tree, f, indent=2)
    
    print(f"\nTree rebuilt: {len(tree_nodes)} → {len(updated_tree)} nodes", file=sys.stderr)
    print("Done!", file=sys.stderr)

if __name__ == '__main__':
    main()
