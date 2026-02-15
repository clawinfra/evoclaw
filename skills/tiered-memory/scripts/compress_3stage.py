#!/usr/bin/env python3
"""
Three-stage compression for tiered memory:
Stage 1 (Raw) → Stage 2 (Distilled) → Stage 3 (Core Summary)
"""

import json
import sys
import urllib.request

def stage1_to_stage2(raw_text, llm_endpoint=None, api_key=None):
    """
    Stage 1 → 2: Raw conversation → Distilled fact (80 bytes target).
    
    Uses LLM to extract structured info, or rule-based extraction.
    """
    if llm_endpoint:
        return llm_distill(raw_text, llm_endpoint, api_key)
    else:
        return rule_distill(raw_text)

def stage2_to_stage3(distilled_fact, llm_endpoint=None, api_key=None):
    """
    Stage 2 → 3: Distilled fact → Core summary (20 bytes target).
    
    Generates ultra-compact summary for tree index.
    """
    if llm_endpoint:
        prompt = f"""Extract a one-line summary (max 20 words) from this fact:

{distilled_fact}

Output ONLY the summary text, nothing else."""

        try:
            headers = {
                'Content-Type': 'application/json',
            }
            if api_key:
                headers['Authorization'] = f'Bearer {api_key}'
            
            data = {
                'messages': [{'role': 'user', 'content': prompt}],
                'max_tokens': 50,
                'temperature': 0.3
            }
            
            req = urllib.request.Request(
                llm_endpoint,
                data=json.dumps(data).encode(),
                headers=headers
            )
            
            with urllib.request.urlopen(req, timeout=10) as response:
                result = json.loads(response.read())
                
                if 'choices' in result:
                    summary = result['choices'][0]['message']['content'].strip()
                elif 'text' in result:
                    summary = result['text'].strip()
                else:
                    raise ValueError("Unknown response format")
                
                # Truncate to 20 words max
                words = summary.split()[:20]
                return ' '.join(words)
                
        except Exception as e:
            print(f"LLM compression failed: {e}, using rule-based", file=sys.stderr)
            return rule_compress(distilled_fact)
    else:
        return rule_compress(distilled_fact)

def llm_distill(raw_text, llm_endpoint, api_key=None):
    """LLM-powered stage 1→2 distillation."""
    prompt = f"""Extract key facts from this conversation. Format as JSON:

{raw_text}

Output format:
{{
  "fact": "one sentence summary",
  "category": "suggested category path",
  "importance": 0.5,
  "people": ["name1", "name2"],
  "topics": ["topic1", "topic2"],
  "actions": ["action taken"],
  "outcome": "positive/negative/neutral"
}}

Keep fact under 80 bytes."""

    try:
        headers = {
            'Content-Type': 'application/json',
        }
        if api_key:
            headers['Authorization'] = f'Bearer {api_key}'
        
        data = {
            'messages': [{'role': 'user', 'content': prompt}],
            'max_tokens': 200,
            'temperature': 0.3
        }
        
        req = urllib.request.Request(
            llm_endpoint,
            data=json.dumps(data).encode(),
            headers=headers
        )
        
        with urllib.request.urlopen(req, timeout=15) as response:
            result = json.loads(response.read())
            
            if 'choices' in result:
                content = result['choices'][0]['message']['content']
            elif 'text' in result:
                content = result['text']
            else:
                raise ValueError("Unknown response format")
            
            distilled = json.loads(content.strip())
            return distilled['fact']
            
    except Exception as e:
        print(f"LLM distillation failed: {e}, using rules", file=sys.stderr)
        return rule_distill(raw_text)

def rule_distill(raw_text):
    """Rule-based stage 1→2 distillation (fallback)."""
    # Simple extraction: first sentence or first 80 chars
    sentences = raw_text.split('. ')
    if sentences:
        fact = sentences[0].strip()
        if len(fact) > 80:
            fact = fact[:77] + '...'
        return fact
    else:
        return raw_text[:80]

def rule_compress(distilled_fact):
    """Rule-based stage 2→3 compression (fallback)."""
    # Extract key words (nouns, verbs)
    import re
    words = re.findall(r'\b[a-z]{3,}\b', distilled_fact.lower())
    
    # Take first 5-7 significant words
    summary_words = []
    for word in words:
        if word not in {'the', 'and', 'for', 'with', 'this', 'that', 'from', 'have', 'been'}:
            summary_words.append(word)
        if len(summary_words) >= 7:
            break
    
    summary = ' '.join(summary_words)
    if len(summary) > 50:
        summary = summary[:47] + '...'
    
    return summary

def compress_pipeline(raw_text, llm_endpoint=None, api_key=None):
    """
    Full 3-stage compression pipeline.
    
    Returns:
        {
          'raw_bytes': int,
          'stage2': str,
          'stage2_bytes': int,
          'stage3': str,
          'stage3_bytes': int,
          'compression_ratio': float
        }
    """
    raw_bytes = len(raw_text.encode())
    
    # Stage 1 → 2
    stage2 = stage1_to_stage2(raw_text, llm_endpoint, api_key)
    stage2_bytes = len(stage2.encode())
    
    # Stage 2 → 3
    stage3 = stage2_to_stage3(stage2, llm_endpoint, api_key)
    stage3_bytes = len(stage3.encode())
    
    compression_ratio = raw_bytes / max(stage3_bytes, 1)
    
    return {
        'raw_bytes': raw_bytes,
        'stage2': stage2,
        'stage2_bytes': stage2_bytes,
        'stage3': stage3,
        'stage3_bytes': stage3_bytes,
        'compression_ratio': compression_ratio
    }

def main():
    import argparse
    parser = argparse.ArgumentParser(description='3-stage memory compression')
    parser.add_argument('--text', help='Text to compress (or read from stdin)')
    parser.add_argument('--llm-endpoint', help='LLM API endpoint')
    parser.add_argument('--api-key', help='API key')
    
    args = parser.parse_args()
    
    # Get input text
    if args.text:
        raw_text = args.text
    else:
        raw_text = sys.stdin.read()
    
    # Run compression
    result = compress_pipeline(raw_text, args.llm_endpoint, args.api_key)
    
    # Output
    print(json.dumps(result, indent=2))

if __name__ == '__main__':
    main()
