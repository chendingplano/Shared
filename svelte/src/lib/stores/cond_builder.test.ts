/**
 * Test and example usage of parseCondition function
 */

import { parseCondition } from './cond_builder';

// Example 1: Simple AND condition
const cond1 = parseCondition("field1 = 'xxx' AND field2 > 100");
console.log("Example 1:", JSON.stringify(cond1, null, 2));
// Result: AND condition with two atomic conditions

// Example 2: Simple OR condition
const cond2 = parseCondition("status = 'active' OR status = 'pending'");
console.log("Example 2:", JSON.stringify(cond2, null, 2));

// Example 3: Complex nested condition with parentheses
const cond3 = parseCondition("field1 = 'xxx' AND (field2 > 100 OR field3 < 50)");
console.log("Example 3:", JSON.stringify(cond3, null, 2));

// Example 4: Using different operators
const cond4 = parseCondition("age >= 18 AND name CONTAINS 'John' AND score <= 100");
console.log("Example 4:", JSON.stringify(cond4, null, 2));

// Example 5: Using && and || operators
const cond5 = parseCondition("field1 == 'xxx' && field2 > 100 || field3 = 'yyy'");
console.log("Example 5:", JSON.stringify(cond5, null, 2));

// Example 6: Your specific example
const cond6 = parseCondition("status = 'in_progress' AND app_type = 'tax_return' OR status = 'completed'");
console.log("Example 6:", JSON.stringify(cond6, null, 2));
// This creates: (status = 'in_progress' AND app_type = 'tax_return') OR (status = 'completed')

// Example 7: Explicit grouping for different precedence
const cond7 = parseCondition("status = 'in_progress' AND (app_type = 'tax_return' OR status = 'completed')");
console.log("Example 7:", JSON.stringify(cond7, null, 2));
// This creates: status = 'in_progress' AND (app_type = 'tax_return' OR status = 'completed')
