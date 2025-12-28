/////////////////////////////////////////////////
// Description:
// condition_constrctor is used to construct conditions.
// Below are some examples of using condition_constructor
// to construct conditions.
//
// Example 1: OR with nested AND groups
// const cond1 = cond_builder.or()
//     .addCond(
//        cond_builder.and()
//           .condEq(...)
//           .condNe(...)
//           ...
//     )
//     .condEq('role', 'admin')   // Simple condition (i.e. not nested)
//     .build();
//
// This creates:
// {
//   type: 'or',
//   conditions: [
//     {
//       type: 'and',
//       conditions: [
//         { type: 'atomic', fieldName: 'status', opr: 'equal', value: 'active', dataType: 'string' },
//         { type: 'atomic', fieldName: 'age', opr: 'greater_than', value: 18, dataType: 'number' }
//       ]
//     },
//     {
//       type: 'and',
//       conditions: [
//         { type: 'atomic', fieldName: 'role', opr: 'equal', value: 'admin', dataType: 'string' },
//         { type: 'atomic', fieldName: 'department', opr: 'equal', value: 'IT', dataType: 'string' }
//       ]
//     }
//   ]
// }
//
// Example 2: More complex nested condition
// const cond2 = condition_constructor.and()
//     .condEq('status', 'active')
//     .or()
//         .condEq('role', 'admin')
//         .condGt('points', 1000)
//     .build();
//
// Example 3: Mixed conditions
// const cond3 = condition_constructor.or()
//     .and()
//         .condEq('status', 'pending')
//         .condLt('priority', 5)
//     .and()
//         .condEq('status', 'active')
//         .condContains('tags', 'important')
//     .build();
// 
// Created: 2025/12/14 by Chen Ding (Qwen generated)
/////////////////////////////////////////////////

import type { CondDef, NullCondition } from "$lib/types/CommonTypes";

// Base class for building conditions
class ConditionBuilder {
    protected conditions: CondDef[] = [];

    // Add a nested condition (another builder or pre-built condition)
    addCond(condition: ConditionBuilder | CondDef): this {
        if (condition instanceof ConditionBuilder) {
            this.conditions.push(condition.build());
        } else {
            this.conditions.push(condition);
        }
        return this;
    }

    // Add an atomic equal condition
    condEq(field_name: string, value: any, data_type: string = 'string'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: '=',
            value,
            data_type
        });
        return this;
    }

    // Add an atomic greater than condition
    condGt(field_name: string, value: any, data_type: string = 'number'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: '>',
            value,
            data_type
        });
        return this;
    }

    // Add an atomic greater than or equal condition
    condGte(field_name: string, value: any, data_type: string = 'number'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: '>=',
            value,
            data_type
        });
        return this;
    }

    // Add an atomic less than condition
    condLt(field_name: string, value: any, data_type: string = 'number'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: '<',
            value,
            data_type 
        });
        return this;
    }

    // Add an atomic less than or equal condition
    condLte(field_name: string, value: any, data_type: string = 'number'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: '<=',
            value,
            data_type 
        });
        return this;
    }

    // Add an atomic not equal condition
    condNe(field_name: string, value: any, data_type: string = 'string'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: '<>',
            value,
            data_type 
        });
        return this;
    }

    // Add an atomic contains condition
    condContains(field_name: string, value: any, data_type: string = 'string'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: 'contain',
            value,
            data_type
        });
        return this;
    }

    // Add an atomic starts with condition
    condPrefix(field_name: string, value: any, data_type: string = 'string'): this {
        this.conditions.push({
            type: 'atomic',
            field_name,
            opr: 'prefix',
            value,
            data_type 
        });
        return this;
    }

    // Build the final condition object
    build(): CondDef {
        if (this.conditions.length === 1) {
            return this.conditions[0];
        }
        // If multiple conditions are added without grouping, treat as AND
        return {
            type: 'and',
            conditions: this.conditions
        };
    }
}

// AND condition builder
class AndCondition extends ConditionBuilder {
    constructor() {
        super();
    }

    override build(): CondDef {
        if (this.conditions.length === 1) {
            return this.conditions[0];
        }
        return {
            type: 'and',
            conditions: this.conditions
        };
    }
}

// OR condition builder
class OrCondition extends ConditionBuilder {
    constructor() {
        super();
    }

    override build(): CondDef {
        if (this.conditions.length === 1) {
            return this.conditions[0];
        }
        return {
            type: 'or',
            conditions: this.conditions
        };
    }
}

// Main constructor class
class ConditionConstructor {
    // Start building an OR condition
    or(): OrCondition {
        return new OrCondition();
    }

    // Start building an AND condition
    and(): AndCondition {
        return new AndCondition();
    }

    filter(): AndCondition {
        return new AndCondition();
    }

    null(): NullCondition {
        return {
            type:       'null',
        };
    }
}

// Global instance for easy access
const cond_builder = new ConditionConstructor();

/////////////////////////////////////////////////
// String Condition Parser
/////////////////////////////////////////////////

// Token types for lexer
enum TokenType {
    FIELD = 'FIELD',
    OPERATOR = 'OPERATOR',
    VALUE = 'VALUE',
    AND = 'AND',
    OR = 'OR',
    LPAREN = 'LPAREN',
    RPAREN = 'RPAREN',
    EOF = 'EOF'
}

interface Token {
    type: TokenType;
    value: string;
}

// Tokenize the input string
function tokenize(input: string): Token[] {
    const tokens: Token[] = [];
    let i = 0;

    // Normalize operators: convert == to =, && to AND, || to OR
    input = input.replace(/==/g, '=')
                 .replace(/&&/g, ' AND ')
                 .replace(/\|\|/g, ' OR ');

    while (i < input.length) {
        // Skip whitespace
        if (/\s/.test(input[i])) {
            i++;
            continue;
        }

        // Left parenthesis
        if (input[i] === '(') {
            tokens.push({ type: TokenType.LPAREN, value: '(' });
            i++;
            continue;
        }

        // Right parenthesis
        if (input[i] === ')') {
            tokens.push({ type: TokenType.RPAREN, value: ')' });
            i++;
            continue;
        }

        // String value (quoted)
        if (input[i] === '"' || input[i] === "'") {
            const quote = input[i];
            let value = '';
            i++; // skip opening quote
            while (i < input.length && input[i] !== quote) {
                if (input[i] === '\\' && i + 1 < input.length) {
                    // Handle escaped characters
                    i++;
                    value += input[i];
                } else {
                    value += input[i];
                }
                i++;
            }
            i++; // skip closing quote
            tokens.push({ type: TokenType.VALUE, value });
            continue;
        }

        // Operators: >=, <=, !=, >, <, =, CONTAINS, PREFIX
        if (input.substring(i, i + 2) === '>=' || input.substring(i, i + 2) === '<=' || input.substring(i, i + 2) === '!=') {
            tokens.push({ type: TokenType.OPERATOR, value: input.substring(i, i + 2) });
            i += 2;
            continue;
        }

        if (input[i] === '>' || input[i] === '<' || input[i] === '=') {
            tokens.push({ type: TokenType.OPERATOR, value: input[i] });
            i++;
            continue;
        }

        // Keywords and identifiers
        if (/[a-zA-Z_]/.test(input[i])) {
            let word = '';
            while (i < input.length && /[a-zA-Z0-9_]/.test(input[i])) {
                word += input[i];
                i++;
            }

            const upperWord = word.toUpperCase();
            if (upperWord === 'AND') {
                tokens.push({ type: TokenType.AND, value: 'AND' });
            } else if (upperWord === 'OR') {
                tokens.push({ type: TokenType.OR, value: 'OR' });
            } else if (upperWord === 'CONTAINS') {
                tokens.push({ type: TokenType.OPERATOR, value: 'CONTAINS' });
            } else if (upperWord === 'PREFIX') {
                tokens.push({ type: TokenType.OPERATOR, value: 'PREFIX' });
            } else {
                // Field name (before operator) or unquoted value (after operator)
                // We'll determine this in the parser
                tokens.push({ type: TokenType.FIELD, value: word });
            }
            continue;
        }

        // Numbers
        if (/[0-9.-]/.test(input[i])) {
            let num = '';
            while (i < input.length && /[0-9.]/.test(input[i])) {
                num += input[i];
                i++;
            }
            tokens.push({ type: TokenType.VALUE, value: num });
            continue;
        }

        // Unknown character
        throw new Error(`Unexpected character at position ${i}: ${input[i]}`);
    }

    tokens.push({ type: TokenType.EOF, value: '' });
    return tokens;
}

// Parse tokens into a CondDef
class ConditionParser {
    private tokens: Token[];
    private current = 0;

    constructor(tokens: Token[]) {
        this.tokens = tokens;
    }

    parse(): CondDef {
        return this.parseOrExpression();
    }

    private parseOrExpression(): CondDef {
        let left = this.parseAndExpression();

        while (this.match(TokenType.OR)) {
            const builder = cond_builder.or().addCond(left);

            do {
                const right = this.parseAndExpression();
                builder.addCond(right);
            } while (this.match(TokenType.OR));

            return builder.build();
        }

        return left;
    }

    private parseAndExpression(): CondDef {
        let left = this.parsePrimary();

        while (this.match(TokenType.AND)) {
            const builder = cond_builder.and().addCond(left);

            do {
                const right = this.parsePrimary();
                builder.addCond(right);
            } while (this.match(TokenType.AND));

            return builder.build();
        }

        return left;
    }

    private parsePrimary(): CondDef {
        // Handle parenthesized expressions
        if (this.match(TokenType.LPAREN)) {
            const expr = this.parseOrExpression();
            this.consume(TokenType.RPAREN, "Expected ')'");
            return expr;
        }

        // Parse atomic condition: field_name operator value
        const fieldToken = this.consume(TokenType.FIELD, "Expected field name");
        const operatorToken = this.consume(TokenType.OPERATOR, "Expected operator");
        const valueToken = this.peek();

        let value: any;
        let dataType: string;

        if (valueToken.type === TokenType.VALUE) {
            this.advance();
            // Determine data type from value
            if (valueToken.value.match(/^-?\d+$/)) {
                value = parseInt(valueToken.value);
                dataType = 'number';
            } else if (valueToken.value.match(/^-?\d+\.\d+$/)) {
                value = parseFloat(valueToken.value);
                dataType = 'number';
            } else {
                value = valueToken.value;
                dataType = 'string';
            }
        } else if (valueToken.type === TokenType.FIELD) {
            // Unquoted value (treat as string)
            this.advance();
            value = valueToken.value;
            dataType = 'string';
        } else {
            throw new Error(`Expected value after operator, got ${valueToken.type}`);
        }

        // Map operator to builder method
        const builder = cond_builder.filter();
        const operator = operatorToken.value.toUpperCase();

        switch (operator) {
            case '=':
                builder.condEq(fieldToken.value, value, dataType);
                break;
            case '!=':
                builder.condNe(fieldToken.value, value, dataType);
                break;
            case '>':
                builder.condGt(fieldToken.value, value, dataType);
                break;
            case '>=':
                builder.condGte(fieldToken.value, value, dataType);
                break;
            case '<':
                builder.condLt(fieldToken.value, value, dataType);
                break;
            case '<=':
                builder.condLte(fieldToken.value, value, dataType);
                break;
            case 'CONTAINS':
                builder.condContains(fieldToken.value, value, dataType);
                break;
            case 'PREFIX':
                builder.condPrefix(fieldToken.value, value, dataType);
                break;
            default:
                throw new Error(`Unknown operator: ${operator}`);
        }

        return builder.build();
    }

    private match(type: TokenType): boolean {
        if (this.check(type)) {
            this.advance();
            return true;
        }
        return false;
    }

    private check(type: TokenType): boolean {
        if (this.isAtEnd()) return false;
        return this.peek().type === type;
    }

    private advance(): Token {
        if (!this.isAtEnd()) this.current++;
        return this.previous();
    }

    private isAtEnd(): boolean {
        return this.peek().type === TokenType.EOF;
    }

    private peek(): Token {
        return this.tokens[this.current];
    }

    private previous(): Token {
        return this.tokens[this.current - 1];
    }

    private consume(type: TokenType, message: string): Token {
        if (this.check(type)) return this.advance();

        const current = this.peek();
        throw new Error(`${message} at position ${this.current}, got ${current.type}: ${current.value}`);
    }
}

/**
 * Parse a string condition expression into a CondDef object.
 *
 * Supported operators:
 * - Comparison: =, ==, !=, >, >=, <, <=
 * - String: CONTAINS, PREFIX
 *
 * Logical operators:
 * - AND, &&
 * - OR, ||
 *
 * Grouping with parentheses: ( )
 *
 * Examples:
 * - "field1 = 'value' AND field2 > 10"
 * - "status = 'active' OR (role = 'admin' AND level >= 5)"
 * - "name CONTAINS 'John' && age > 30"
 *
 * @param condition - The condition string to parse
 * @returns A CondDef object that can be used with db_store
 */
export function parseCondition(condition: string): CondDef {
    const tokens = tokenize(condition);
    const parser = new ConditionParser(tokens);
    return parser.parse();
}

export {
    cond_builder,
    ConditionConstructor,
    AndCondition,
    OrCondition
};