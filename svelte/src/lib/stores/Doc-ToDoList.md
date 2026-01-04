# Query Builder - To-Do List

## Create Custom Filters
1. Let users create custom query_name
2. Need to support the ternary operator. When the condition is not true, it is 'undefined', which will not generate the condition at all.

Source: https://orm.drizzle.team/docs/guides/conditional-filters-in-query

```typescript
// length less than
const lenlt = (column: AnyColumn, value: number) => {
  return sql`length(${column}) < ${value}`;
};
const searchPosts = async (maxLen = 0, views = 0) => {
  await db
    .select()
    .from(posts)
    .where(
      and(
        maxLen ? lenlt(posts.title, maxLen) : undefined,
        views > 100 ? gt(posts.views, views) : undefined,
      ),
    );
};
await searchPosts(8);
await searchPosts(8, 200);
```

